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

// HandleRemoteMulticastSetupCommand handles an uplink remote multicast setup command.
func HandleRemoteMulticastSetupCommand(db sqlx.Ext, devEUI lorawan.EUI64, b []byte) error {
	var cmd multicastsetup.Command

	if err := cmd.UnmarshalBinary(true, b); err != nil {
		errors.Wrap(err, "unmarshal command error")
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
