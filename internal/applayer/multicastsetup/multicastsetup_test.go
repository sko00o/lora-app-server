package multicastsetup

import (
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/lora-app-server/internal/config"
	"github.com/brocaar/lora-app-server/internal/storage"
	"github.com/brocaar/lora-app-server/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/applayer/multicastsetup"
)

type MulticastSetupTestSuite struct {
	suite.Suite
	test.DatabaseTestSuiteBase

	NSClient         *test.NetworkServerClient
	NetworkServer    storage.NetworkServer
	Organization     storage.Organization
	ServiceProfile   storage.ServiceProfile
	Application      storage.Application
	DeviceProfile    storage.DeviceProfile
	Device           storage.Device
	DeviceActivation storage.DeviceActivation
	MulticastGroup   storage.MulticastGroup
}

func (ts *MulticastSetupTestSuite) SetupSuite() {
	ts.DatabaseTestSuiteBase.SetupSuite()

	config.C.ApplicationServer.RemoteMulticastSetup.SyncInterval = time.Minute
	config.C.ApplicationServer.RemoteMulticastSetup.SyncRetries = 5
	config.C.ApplicationServer.RemoteMulticastSetup.BatchSize = 10
}

func (ts *MulticastSetupTestSuite) SetupTest() {
	ts.DatabaseTestSuiteBase.SetupTest()

	assert := require.New(ts.T())

	ts.NSClient = test.NewNetworkServerClient()
	config.C.NetworkServer.Pool = test.NewNetworkServerPool(ts.NSClient)

	ts.NetworkServer = storage.NetworkServer{
		Name:   "test",
		Server: "test:1234",
	}
	assert.NoError(storage.CreateNetworkServer(ts.Tx(), &ts.NetworkServer))

	ts.Organization = storage.Organization{
		Name: "test-org",
	}
	assert.NoError(storage.CreateOrganization(ts.Tx(), &ts.Organization))

	ts.ServiceProfile = storage.ServiceProfile{
		Name:            "test-sp",
		OrganizationID:  ts.Organization.ID,
		NetworkServerID: ts.NetworkServer.ID,
	}
	assert.NoError(storage.CreateServiceProfile(ts.Tx(), &ts.ServiceProfile))
	var spID uuid.UUID
	copy(spID[:], ts.ServiceProfile.ServiceProfile.Id)

	ts.Application = storage.Application{
		Name:             "test-app",
		OrganizationID:   ts.Organization.ID,
		ServiceProfileID: spID,
	}
	assert.NoError(storage.CreateApplication(ts.Tx(), &ts.Application))

	ts.DeviceProfile = storage.DeviceProfile{
		Name:            "test-dp",
		OrganizationID:  ts.Organization.ID,
		NetworkServerID: ts.NetworkServer.ID,
	}
	assert.NoError(storage.CreateDeviceProfile(ts.Tx(), &ts.DeviceProfile))
	var dpID uuid.UUID
	copy(dpID[:], ts.DeviceProfile.DeviceProfile.Id)

	ts.Device = storage.Device{
		DevEUI:          lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		ApplicationID:   ts.Application.ID,
		DeviceProfileID: dpID,
		Name:            "test-device",
		Description:     "test device",
	}
	assert.NoError(storage.CreateDevice(ts.Tx(), &ts.Device))

	ts.DeviceActivation = storage.DeviceActivation{
		DevEUI: ts.Device.DevEUI,
	}
	assert.NoError(storage.CreateDeviceActivation(ts.Tx(), &ts.DeviceActivation))

	ts.MulticastGroup = storage.MulticastGroup{
		Name:             "test-mg",
		MCAppSKey:        lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		MCKey:            lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		ServiceProfileID: spID,
	}
	assert.NoError(storage.CreateMulticastGroup(ts.Tx(), &ts.MulticastGroup))
}

func (ts *MulticastSetupTestSuite) TestSyncRemoteMulticastSetupReq() {
	assert := require.New(ts.T())
	ms := storage.RemoteMulticastSetup{
		DevEUI:         ts.Device.DevEUI,
		McGroupID:      1,
		McAddr:         lorawan.DevAddr{1, 2, 3, 4},
		McKeyEncrypted: lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		MinMcFCnt:      10,
		MaxMcFCnt:      20,
		State:          storage.RemoteMulticastSetupSetup,
	}
	copy(ms.MulticastGroupID[:], ts.MulticastGroup.MulticastGroup.Id)

	assert.NoError(storage.CreateRemoteMulticastSetup(ts.Tx(), &ms))
	assert.NoError(syncRemoteMulticastSetup(ts.Tx()))

	ms, err := storage.GetRemoteMulticastSetup(ts.Tx(), ms.DevEUI, ms.MulticastGroupID, false)
	assert.NoError(err)
	assert.Equal(1, ms.RetryCount)
	assert.True(ms.RetryAfter.After(time.Now()))

	req := <-ts.NSClient.CreateDeviceQueueItemChan
	assert.Equal(multicastsetup.DefaultFPort, uint8(req.Item.FPort))

	b, err := lorawan.EncryptFRMPayload(ts.DeviceActivation.AppSKey, false, ts.DeviceActivation.DevAddr, 0, req.Item.FrmPayload)
	assert.NoError(err)

	var cmd multicastsetup.Command
	assert.NoError(cmd.UnmarshalBinary(false, b))

	assert.Equal(multicastsetup.Command{
		CID: multicastsetup.McGroupSetupReq,
		Payload: &multicastsetup.McGroupSetupReqPayload{
			McGroupIDHeader: multicastsetup.McGroupSetupReqPayloadMcGroupIDHeader{
				McGroupID: 1,
			},
			McAddr:         ms.McAddr,
			McKeyEncrypted: ms.McKeyEncrypted,
			MinMcFCnt:      ms.MinMcFCnt,
			MaxMcFCnt:      ms.MaxMcFCnt,
		},
	}, cmd)

}

func (ts *MulticastSetupTestSuite) TestSyncRemoteMulticastDeleteReq() {
	assert := require.New(ts.T())

	ms := storage.RemoteMulticastSetup{
		DevEUI:         ts.Device.DevEUI,
		McGroupID:      1,
		McAddr:         lorawan.DevAddr{1, 2, 3, 4},
		McKeyEncrypted: lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		MinMcFCnt:      10,
		MaxMcFCnt:      20,
		State:          storage.RemoteMulticastSetupDelete,
	}
	copy(ms.MulticastGroupID[:], ts.MulticastGroup.MulticastGroup.Id)

	assert.NoError(storage.CreateRemoteMulticastSetup(ts.Tx(), &ms))
	assert.NoError(syncRemoteMulticastSetup(ts.Tx()))

	ms, err := storage.GetRemoteMulticastSetup(ts.Tx(), ms.DevEUI, ms.MulticastGroupID, false)
	assert.NoError(err)
	assert.Equal(1, ms.RetryCount)
	assert.True(ms.RetryAfter.After(time.Now()))

	req := <-ts.NSClient.CreateDeviceQueueItemChan
	assert.Equal(multicastsetup.DefaultFPort, uint8(req.Item.FPort))

	b, err := lorawan.EncryptFRMPayload(ts.DeviceActivation.AppSKey, false, ts.DeviceActivation.DevAddr, 0, req.Item.FrmPayload)
	assert.NoError(err)

	var cmd multicastsetup.Command
	assert.NoError(cmd.UnmarshalBinary(false, b))

	assert.Equal(multicastsetup.Command{
		CID: multicastsetup.McGroupDeleteReq,
		Payload: &multicastsetup.McGroupDeleteReqPayload{
			McGroupIDHeader: multicastsetup.McGroupDeleteReqPayloadMcGroupIDHeader{
				McGroupID: 1,
			},
		},
	}, cmd)
}

func TestMulticastSetup(t *testing.T) {
	suite.Run(t, new(MulticastSetupTestSuite))
}
