package fragmentation

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lora-app-server/internal/config"
	"github.com/brocaar/lora-app-server/internal/downlink"
	"github.com/brocaar/lora-app-server/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/applayer/fragmentation"
)

// SyncRemoteFragmentationSessions syncs the fragmentation sessions with the devices.
func SyncRemoteFragmentationSessionsLoop() {
	for {
		err := storage.Transaction(config.C.PostgreSQL.DB, func(tx sqlx.Ext) error {
			return syncRemoteFragmentationSessions(tx)
		})
		if err != nil {
			log.WithError(err).Error("sync remote fragmentation setup error")
		}
		time.Sleep(config.C.ApplicationServer.RemoteMulticastSetup.SyncInterval)
	}
}

// HandleRemoteFragmentationSessionCommand handles an uplink fragmentation session command.
func HandleRemoteFragmentationSessionCommand(db sqlx.Ext, devEUI lorawan.EUI64, b []byte) error {
	var cmd fragmentation.Command

	if err := cmd.UnmarshalBinary(true, b); err != nil {
		return errors.Wrap(err, "unmarshal command error")
	}

	switch cmd.CID {
	case fragmentation.FragSessionSetupAns:
		pl, ok := cmd.Payload.(*fragmentation.FragSessionSetupAnsPayload)
		if !ok {
			return fmt.Errorf("expected *fragmentation.FragSessionSetupAnsPayload, got: %T", cmd.Payload)
		}
		if err := handleFragSessionSetupAns(db, devEUI, pl); err != nil {
			return errors.Wrap(err, "handle FragSessionSetupAns error")
		}
	case fragmentation.FragSessionDeleteAns:
		pl, ok := cmd.Payload.(*fragmentation.FragSessionDeleteAnsPayload)
		if !ok {
			return fmt.Errorf("exlected *fragmentation.FragSessionDeleteAnsPayload, got: %T", cmd.Payload)
		}
		if err := handleFragSessionDeleteAns(db, devEUI, pl); err != nil {
			return errors.Wrap(err, "handle FragSessionDeleteAns error")
		}
	default:
		return fmt.Errorf("CID not implemented: %s", cmd.CID)
	}

	return nil
}

func syncRemoteFragmentationSessions(db sqlx.Ext) error {
	items, err := storage.GetPendingRemoteFragmentationSessions(db, config.C.ApplicationServer.RemoteMulticastSetup.SyncBatchSize, config.C.ApplicationServer.RemoteMulticastSetup.SyncRetries)
	if err != nil {
		return err
	}

	for _, item := range items {
		if err := syncRemoteFragmentationSession(db, item); err != nil {
			return errors.Wrap(err, "sync remote fragmentation session error")
		}
	}

	return nil
}

func syncRemoteFragmentationSession(db sqlx.Ext, item storage.RemoteFragmentationSession) error {
	var cmd fragmentation.Command

	switch item.State {
	case storage.RemoteMulticastSetupSetup:
		pl := fragmentation.FragSessionSetupReqPayload{
			FragSession: fragmentation.FragSessionSetupReqPayloadFragSession{
				FragIndex: uint8(item.FragIndex),
			},
			NbFrag:   uint16(item.NbFrag),
			FragSize: uint8(item.FragSize),
			Control: fragmentation.FragSessionSetupReqPayloadControl{
				FragmentationMatrix: item.FragmentationMatrix,
				BlockAckDelay:       uint8(item.BlockAckDelay),
			},
			Padding:    uint8(item.Padding),
			Descriptor: item.Descriptor,
		}

		for _, idx := range item.MCGroupIDs {
			if idx <= 3 {
				pl.FragSession.McGroupBitMask[idx] = true
			}
		}

		cmd = fragmentation.Command{
			CID:     fragmentation.FragSessionSetupReq,
			Payload: &pl,
		}
	case storage.RemoteMulticastSetupDelete:
		cmd = fragmentation.Command{
			CID: fragmentation.FragSessionDeleteReq,
			Payload: &fragmentation.FragSessionDeleteReqPayload{
				Param: fragmentation.FragSessionDeleteReqPayloadParam{
					FragIndex: uint8(item.FragIndex),
				},
			},
		}
	default:
		return fmt.Errorf("invalid state: %s", item.State)
	}

	b, err := cmd.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "marshal binary error")
	}

	_, err = downlink.EnqueueDownlinkPayload(db, item.DevEUI, false, fragmentation.DefaultFPort, b)
	if err != nil {
		return errors.Wrap(err, "enqueue downlink payload error")
	}

	item.RetryCount++
	item.RetryAfter = time.Now().Add(config.C.ApplicationServer.RemoteMulticastSetup.SyncInterval)

	err = storage.UpdateRemoteFragmentationSession(db, &item)
	if err != nil {
		return errors.Wrap(err, "update remote fragmentation session error")
	}

	return nil
}

func handleFragSessionSetupAns(db sqlx.Ext, devEUI lorawan.EUI64, pl *fragmentation.FragSessionSetupAnsPayload) error {
	if pl.StatusBitMask.WrongDescriptor || pl.StatusBitMask.FragSessionIndexNotSupported || pl.StatusBitMask.NotEnoughMemory || pl.StatusBitMask.EncodingUnsupported {
		return fmt.Errorf("WrongDescriptor: %t, FragSessionIndexNotSupported: %t, NotEnoughMemory: %t, EncodingUnsupported: %t", pl.StatusBitMask.WrongDescriptor, pl.StatusBitMask.FragSessionIndexNotSupported, pl.StatusBitMask.NotEnoughMemory, pl.StatusBitMask.EncodingUnsupported)
	}

	rfs, err := storage.GetRemoteFragmentationSession(db, devEUI, int(pl.StatusBitMask.FragIndex), true)
	if err != nil {
		return errors.Wrap(err, "get remote fragmentation session error")
	}

	rfs.StateProvisioned = true
	if err := storage.UpdateRemoteFragmentationSession(db, &rfs); err != nil {
		return errors.Wrap(err, "update remote fragmentation session error")
	}

	return nil
}

func handleFragSessionDeleteAns(db sqlx.Ext, devEUI lorawan.EUI64, pl *fragmentation.FragSessionDeleteAnsPayload) error {
	if pl.Status.SessionDoesNotExist {
		return fmt.Errorf("FragIndex %d does not exist", pl.Status.FragIndex)
	}

	rfs, err := storage.GetRemoteFragmentationSession(db, devEUI, int(pl.Status.FragIndex), true)
	if err != nil {
		return errors.Wrap(err, "get remove fragmentation session error")
	}

	rfs.StateProvisioned = true
	if err := storage.UpdateRemoteFragmentationSession(db, &rfs); err != nil {
		return errors.Wrap(err, "update remote fragmentation session error")
	}

	return nil
}
