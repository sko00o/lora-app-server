package fuota

import (
	"crypto/aes"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lora-app-server/internal/config"
	"github.com/brocaar/lora-app-server/internal/multicast"
	"github.com/brocaar/lora-app-server/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/applayer/fragmentation"
	"github.com/brocaar/lorawan/applayer/multicastsetup"
)

var (
	interval                          = time.Second
	batchSize                         = 1
	mcGroupID                         int
	fragIndex                         int
	remoteMulticastSetupRetries       int
	remoteFragmentationSessionRetries int
)

// Setup configures the package.
func Setup(conf config.Config) error {
	mcGroupID = conf.ApplicationServer.FUOTADeployment.McGroupID
	fragIndex = conf.ApplicationServer.FUOTADeployment.FragIndex
	remoteMulticastSetupRetries = conf.ApplicationServer.RemoteMulticastSetup.SyncRetries
	remoteFragmentationSessionRetries = conf.ApplicationServer.FragmentationSession.SyncRetries

	go fuotaDeploymentLoop()

	return nil
}

func fuotaDeploymentLoop() {
	for {
		err := storage.Transaction(func(tx sqlx.Ext) error {
			return fuotaDeployments(tx)
		})
		if err != nil {
			log.WithError(err).Error("fuota deployment error")
		}
		time.Sleep(interval)
	}
}

func fuotaDeployments(db sqlx.Ext) error {
	items, err := storage.GetPendingFUOTADeployments(db, batchSize)
	if err != nil {
		return err
	}

	for _, item := range items {
		if err := fuotaDeployment(db, item); err != nil {
			return errors.Wrap(err, "fuota deployment error")
		}
	}

	return nil
}

func fuotaDeployment(db sqlx.Ext, item storage.FUOTADeployment) error {
	switch item.State {
	case storage.FUOTADeploymentMulticastSetup:
		return stepMulticastSetup(db, item)
	case storage.FUOTADeploymentFragmentationSessSetup:
		return stepFragmentationSessSetup(db, item)
	case storage.FUOTADeploymentMulticastSessCSetup:
		return stepMulticastSessCSetup(db, item)
	case storage.FUOTADeploymentEnqueue:
		return stepEnqueue(db, item)
	default:
		return fmt.Errorf("unexpected state: %s", item.State)
	}
}

func stepMulticastSetup(db sqlx.Ext, item storage.FUOTADeployment) error {
	mcg, err := storage.GetMulticastGroup(db, *item.MulticastGroupID, false, false)
	if err != nil {
		return errors.Wrap(err, "get multicast group error")
	}

	// query all device-keys that relate to this FUOTA deployment
	var deviceKeys []storage.DeviceKeys
	err = sqlx.Select(db, &deviceKeys, `
		select
			dk.*
		from
			fuota_deployment_device dd
		inner join
			device_keys dk
			on dd.dev_eui = dk.dev_eui
		where
			dd.fuota_deployment_id = $1`,
		item.ID,
	)
	if err != nil {
		return errors.Wrap(err, "sql select error")
	}

	for _, dk := range deviceKeys {
		var nullKey lorawan.AES128Key

		// get the encrypted McKey.
		var mcKeyEncrypted, mcRootKey lorawan.AES128Key
		if dk.AppKey != nullKey {
			mcRootKey, err = multicastsetup.GetMcRootKeyForAppKey(dk.AppKey)
			if err != nil {
				return errors.Wrap(err, "get McRootKey for AppKey error")
			}
		} else {
			mcRootKey, err = multicastsetup.GetMcRootKeyForGenAppKey(dk.GenAppKey)
			if err != nil {
				return errors.Wrap(err, "get McRootKey for GenAppKey error")
			}
		}

		mcKEKey, err := multicastsetup.GetMcKEKey(mcRootKey)
		if err != nil {
			return errors.Wrap(err, "get McKEKey error")
		}

		block, err := aes.NewCipher(mcKEKey[:])
		if err != nil {
			return errors.Wrap(err, "new cipher error")
		}
		block.Encrypt(mcKeyEncrypted[:], mcg.MCKey[:])

		// create remote multicast setup record for device
		rms := storage.RemoteMulticastSetup{
			DevEUI:           dk.DevEUI,
			MulticastGroupID: *item.MulticastGroupID,
			McGroupID:        mcGroupID,
			McKeyEncrypted:   mcKeyEncrypted,
			MinMcFCnt:        0,
			MaxMcFCnt:        (1 << 32) - 1,
			State:            storage.RemoteMulticastSetupSetup,
			RetryInterval:    item.UnicastTimeout,
		}
		copy(rms.McAddr[:], mcg.MulticastGroup.McAddr)

		err = storage.CreateRemoteMulticastSetup(db, &rms)
		if err != nil {
			return errors.Wrap(err, "create remote multicast setup error")
		}
	}

	item.State = storage.FUOTADeploymentFragmentationSessSetup
	item.NextStepAfter = time.Now().Add(time.Duration(remoteMulticastSetupRetries) * item.UnicastTimeout)

	err = storage.UpdateFUOTADeployment(db, &item)
	if err != nil {
		return errors.Wrap(err, "update fuota deployment error")
	}

	return nil
}

func stepFragmentationSessSetup(db sqlx.Ext, item storage.FUOTADeployment) error {
	if item.FragSize == 0 {
		return errors.New("FragSize must not be 0")
	}

	// query all devices with complete multicast setup
	var devEUIs []lorawan.EUI64
	err := sqlx.Select(db, &devEUIs, `
		select
			dev_eui
		from
			remote_multicast_setup
		where
			multicast_group_id = $1
			and state = $2
			and state_provisioned = $3`,
		item.MulticastGroupID,
		storage.RemoteMulticastSetupSetup,
		true,
	)
	if err != nil {
		return errors.Wrap(err, "get devices with multicast setup error")
	}

	padding := len(item.Payload) % item.FragSize
	nbFrag := ((len(item.Payload) + padding) / item.FragSize) + item.Redundancy

	for _, devEUI := range devEUIs {
		// delete existing fragmentation session if it exist
		err = storage.DeleteRemoteFragmentationSession(db, devEUI, fragIndex)
		if err != nil && err != storage.ErrDoesNotExist {
			return errors.Wrap(err, "delete remote fragmentation session error")
		}

		fs := storage.RemoteFragmentationSession{
			DevEUI:              devEUI,
			FragIndex:           fragIndex,
			MCGroupIDs:          []int{mcGroupID},
			NbFrag:              nbFrag,
			FragSize:            item.FragSize,
			FragmentationMatrix: item.FragmentationMatrix,
			BlockAckDelay:       item.BlockAckDelay,
			Padding:             padding,
			Descriptor:          item.Descriptor,
			State:               storage.RemoteMulticastSetupSetup,
			RetryInterval:       item.UnicastTimeout,
		}
		err = storage.CreateRemoteFragmentationSession(db, &fs)
		if err != nil {
			return errors.Wrap(err, "create remote fragmentation session error")
		}
	}

	item.State = storage.FUOTADeploymentMulticastSessCSetup
	item.NextStepAfter = time.Now().Add(time.Duration(remoteFragmentationSessionRetries) * item.UnicastTimeout)

	err = storage.UpdateFUOTADeployment(db, &item)
	if err != nil {
		return errors.Wrap(err, "update fuota deployment error")
	}

	return nil
}

func stepMulticastSessCSetup(db sqlx.Ext, item storage.FUOTADeployment) error {
	mcg, err := storage.GetMulticastGroup(db, *item.MulticastGroupID, false, false)
	if err != nil {
		return errors.Wrap(err, "get multicast group error")
	}

	// query all devices with complete fragmentation session setup
	var devEUIs []lorawan.EUI64
	err = sqlx.Select(db, &devEUIs, `
		select
			rms.dev_eui
		from
			remote_multicast_setup rms
		inner join
			remote_fragmentation_session rfs
		on
			rfs.dev_eui = rms.dev_eui
			and rfs.frag_index = $1
		where
			rms.multicast_group_id = $2
			and rms.state = $3
			and rms.state_provisioned = $4
			and rfs.state = $3
			and rms.state_provisioned = $4`,
		fragIndex,
		item.MulticastGroupID,
		storage.RemoteMulticastSetupSetup,
		true,
	)
	if err != nil {
		return errors.Wrap(err, "get devices with fragmentation session setup error")
	}

	for _, devEUI := range devEUIs {
		rmccs := storage.RemoteMulticastClassCSession{
			DevEUI:           devEUI,
			MulticastGroupID: *item.MulticastGroupID,
			McGroupID:        mcGroupID,
			DLFrequency:      int(mcg.MulticastGroup.Frequency),
			DR:               int(mcg.MulticastGroup.Dr),
			SessionTime:      time.Now().Add(time.Duration(remoteMulticastSetupRetries) * item.UnicastTimeout),
			SessionTimeOut:   item.MulticastTimeout,
			RetryInterval:    item.UnicastTimeout,
		}
		err = storage.CreateRemoteMulticastClassCSession(db, &rmccs)
		if err != nil {
			return errors.Wrap(err, "create remote multicast class-c session error")
		}
	}

	item.State = storage.FUOTADeploymentEnqueue
	item.NextStepAfter = time.Now().Add(time.Duration(remoteMulticastSetupRetries) * item.UnicastTimeout)

	err = storage.UpdateFUOTADeployment(db, &item)
	if err != nil {
		return errors.Wrap(err, "update fuota deployment error")
	}

	return nil
}

func stepEnqueue(db sqlx.Ext, item storage.FUOTADeployment) error {
	// fragment the payload
	fragments, err := fragmentation.Encode(item.Payload, item.FragSize, item.Redundancy)
	if err != nil {
		return errors.Wrap(err, "fragment payload error")
	}

	// wrap the payloads into data-fragment payloads
	var payloads [][]byte
	for i := range fragments {
		cmd := fragmentation.Command{
			CID: fragmentation.DataFragment,
			Payload: &fragmentation.DataFragmentPayload{
				IndexAndN: fragmentation.DataFragmentPayloadIndexAndN{
					FragIndex: uint8(fragIndex),
					N:         uint16(i),
				},
				Payload: fragments[i],
			},
		}
		b, err := cmd.MarshalBinary()
		if err != nil {
			return errors.Wrap(err, "marshal binary error")
		}

		payloads = append(payloads, b)
	}

	// enqueue the payloads
	_, err = multicast.EnqueueMultiple(db, *item.MulticastGroupID, fragmentation.DefaultFPort, payloads)
	if err != nil {
		return errors.Wrap(err, "enqueue multiple error")
	}

	item.State = storage.FUOTADeploymentWaitingTx
	item.NextStepAfter = time.Now().Add(interval)

	err = storage.UpdateFUOTADeployment(db, &item)
	if err != nil {
		return errors.Wrap(err, "update fuota deployment error")
	}

	return nil
}
