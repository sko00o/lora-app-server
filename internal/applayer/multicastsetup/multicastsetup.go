package multicastsetup

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
	"github.com/brocaar/lorawan/applayer/multicastsetup"
	"github.com/brocaar/lorawan/gps"
)

// SyncRemoteMulticastSetupLoop syncs the multicast setup with the devices.
func SyncRemoteMulticastSetupLoop() {
	for {
		err := storage.Transaction(config.C.PostgreSQL.DB, func(tx sqlx.Ext) error {
			return syncRemoteMulticastSetup(tx)
		})

		if err != nil {
			log.WithError(err).Error("sync remote multicast setup error")
		}
		time.Sleep(config.C.ApplicationServer.RemoteMulticastSetup.SyncInterval)
	}
}

// SyncRemoteMulticastClassCSessionLoop syncs the multicast Class-C session
// with the devices.
func SyncRemoteMulticastClassCSessionLoop() {
	for {
		err := storage.Transaction(config.C.PostgreSQL.DB, func(tx sqlx.Ext) error {
			return syncRemoteMulticastClassCSession(tx)
		})

		if err != nil {
			log.WithError(err).Error("sync remote multicast class-c session error")
		}
		time.Sleep(config.C.ApplicationServer.RemoteMulticastSetup.SyncInterval)
	}
}

// HandleRemoteMulticastSetupCommand handles an uplink remote multicast setup command.
func HandleRemoteMulticastSetupCommand(db sqlx.Ext, devEUI lorawan.EUI64, b []byte) error {
	var cmd multicastsetup.Command

	if err := cmd.UnmarshalBinary(true, b); err != nil {
		return errors.Wrap(err, "unmarshal command error")
	}

	switch cmd.CID {
	case multicastsetup.McGroupSetupAns:
		pl, ok := cmd.Payload.(*multicastsetup.McGroupSetupAnsPayload)
		if !ok {
			return fmt.Errorf("expected *multicastsetup.McGroupSetupAnsPayload, got: %T", cmd.Payload)
		}
		if err := handleMcGroupSetupAns(db, devEUI, pl); err != nil {
			return errors.Wrap(err, "handle McGroupSetupAns error")
		}
	case multicastsetup.McClassCSessionAns:
		pl, ok := cmd.Payload.(*multicastsetup.McClassCSessionAnsPayload)
		if !ok {
			return fmt.Errorf("expected *multicastsetup.McClassCSessionAnsPayload, got: %T", cmd.Payload)
		}
		if err := handleMcClassCSessionAns(db, devEUI, pl); err != nil {
			return errors.Wrap(err, "handle McClassCSessionAns error")
		}
	default:
		return fmt.Errorf("CID not implemented: %s", cmd.CID)
	}

	return nil
}

func handleMcGroupSetupAns(db sqlx.Ext, devEUI lorawan.EUI64, pl *multicastsetup.McGroupSetupAnsPayload) error {
	if pl.McGroupIDHeader.IDError {
		return fmt.Errorf("IDError for McGroupID: %d", pl.McGroupIDHeader.McGroupID)
	}

	rms, err := storage.GetRemoteMulticastSetupByGroupID(db, devEUI, int(pl.McGroupIDHeader.McGroupID), true)
	if err != nil {
		return errors.Wrap(err, "get remote multicast-setup by group id error")
	}

	rms.StateProvisioned = true
	if err := storage.UpdateRemoteMulticastSetup(db, &rms); err != nil {
		return errors.Wrap(err, "update remote multicast-setup error")
	}

	return nil
}

func handleMcClassCSessionAns(db sqlx.Ext, devEUI lorawan.EUI64, pl *multicastsetup.McClassCSessionAnsPayload) error {
	if pl.StatusAndMcGroupID.DRError || pl.StatusAndMcGroupID.FreqError || pl.StatusAndMcGroupID.McGroupUndefined {
		return fmt.Errorf("DRError: %t, FreqError: %t, McGroupUndefined: %t for McGroupID: %d", pl.StatusAndMcGroupID.DRError, pl.StatusAndMcGroupID.FreqError, pl.StatusAndMcGroupID.McGroupUndefined, pl.StatusAndMcGroupID.McGroupID)
	}

	sess, err := storage.GetRemoteMulticastClassCSessionByGroupID(db, devEUI, int(pl.StatusAndMcGroupID.McGroupID), true)
	if err != nil {
		return errors.Wrap(err, "get remote multicast class-c session error")
	}

	sess.StateProvisioned = true
	if err := storage.UpdateRemoteMulticastClassCSession(db, &sess); err != nil {
		return errors.Wrap(err, "update remote multicast class-c session error")
	}

	return nil
}

func syncRemoteMulticastSetup(db sqlx.Ext) error {
	items, err := storage.GetPendingRemoteMulticastSetupItems(db, config.C.ApplicationServer.RemoteMulticastSetup.BatchSize, config.C.ApplicationServer.RemoteMulticastSetup.SyncRetries)
	if err != nil {
		return err
	}

	for _, item := range items {
		if err := syncRemoteMulticastSetupItem(db, item); err != nil {
			return errors.Wrap(err, "sync remote multicast-setup error")
		}
	}

	return nil
}

func syncRemoteMulticastSetupItem(db sqlx.Ext, item storage.RemoteMulticastSetup) error {
	var cmd multicastsetup.Command

	switch item.State {
	case storage.RemoteMulticastSetupSetup:
		cmd = multicastsetup.Command{
			CID: multicastsetup.McGroupSetupReq,
			Payload: &multicastsetup.McGroupSetupReqPayload{
				McGroupIDHeader: multicastsetup.McGroupSetupReqPayloadMcGroupIDHeader{
					McGroupID: uint8(item.McGroupID),
				},
				McAddr:         item.McAddr,
				McKeyEncrypted: item.McKeyEncrypted,
				MinMcFCnt:      item.MinMcFCnt,
				MaxMcFCnt:      item.MaxMcFCnt,
			},
		}
	case storage.RemoteMulticastSetupDelete:
		cmd = multicastsetup.Command{
			CID: multicastsetup.McGroupDeleteReq,
			Payload: &multicastsetup.McGroupDeleteReqPayload{
				McGroupIDHeader: multicastsetup.McGroupDeleteReqPayloadMcGroupIDHeader{
					McGroupID: uint8(item.McGroupID),
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

	_, err = downlink.EnqueueDownlinkPayload(db, item.DevEUI, false, multicastsetup.DefaultFPort, b)
	if err != nil {
		return errors.Wrap(err, "enqueue downlink payload error")
	}

	item.RetryCount++
	item.RetryAfter = time.Now().Add(config.C.ApplicationServer.RemoteMulticastSetup.SyncInterval)

	err = storage.UpdateRemoteMulticastSetup(db, &item)
	if err != nil {
		return errors.Wrap(err, "update remote multicast-setup error")
	}

	return nil
}

func syncRemoteMulticastClassCSession(db sqlx.Ext) error {
	items, err := storage.GetPendingRemoteMulticastClassCSessions(db, config.C.ApplicationServer.RemoteMulticastSetup.BatchSize, config.C.ApplicationServer.RemoteMulticastSetup.SyncRetries)
	if err != nil {
		return err
	}

	for _, item := range items {
		if err := syncRemoteMulticastClassCSessionItem(db, item); err != nil {
			return errors.Wrap(err, "sync remote multicast class-c session error")
		}
	}

	return nil
}

func syncRemoteMulticastClassCSessionItem(db sqlx.Ext, item storage.RemoteMulticastClassCSession) error {
	cmd := multicastsetup.Command{
		CID: multicastsetup.McClassCSessionReq,
		Payload: &multicastsetup.McClassCSessionReqPayload{
			McGroupIDHeader: multicastsetup.McClassCSessionReqPayloadMcGroupIDHeader{
				McGroupID: uint8(item.McGroupID),
			},
			SessionTime: uint32((gps.Time(item.SessionTime).TimeSinceGPSEpoch() / time.Second) % (1 << 32)),
			SessionTimeOut: multicastsetup.McClassCSessionReqPayloadSessionTimeOut{
				TimeOut: uint8(item.SessionTimeOut),
			},
			DLFrequency: uint32(item.DLFrequency),
			DR:          uint8(item.DR),
		},
	}

	b, err := cmd.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "marshal binary error")
	}

	_, err = downlink.EnqueueDownlinkPayload(db, item.DevEUI, false, multicastsetup.DefaultFPort, b)
	if err != nil {
		return errors.Wrap(err, "enqueue downlink payload error")
	}

	item.RetryCount++
	item.RetryAfter = time.Now().Add(config.C.ApplicationServer.RemoteMulticastSetup.SyncInterval)

	err = storage.UpdateRemoteMulticastClassCSession(db, &item)
	if err != nil {
		return errors.Wrap(err, "update remote multicast class-c session error")
	}

	return nil
}
