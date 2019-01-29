package multicastsetup

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lora-app-server/internal/config"
	"github.com/brocaar/lora-app-server/internal/downlink"
	"github.com/brocaar/lora-app-server/internal/storage"
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
