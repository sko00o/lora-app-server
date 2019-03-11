package fuota

import (
	"testing"
	"time"

	"github.com/brocaar/lora-app-server/internal/backend/networkserver"
	nsmock "github.com/brocaar/lora-app-server/internal/backend/networkserver/mock"
	"github.com/brocaar/lora-app-server/internal/storage"
	"github.com/brocaar/lora-app-server/internal/test"
	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/applayer/fragmentation"
	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type FUOTATestSuite struct {
	suite.Suite

	tx       *storage.TxLogger
	nsClient *nsmock.Client

	NetworkServer  storage.NetworkServer
	Organization   storage.Organization
	ServiceProfile storage.ServiceProfile
	Application    storage.Application
	DeviceProfile  storage.DeviceProfile
	Device         storage.Device
}

func (ts *FUOTATestSuite) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))
	test.MustResetDB(storage.DB().DB)

	remoteMulticastSetupRetries = 3
	remoteFragmentationSessionRetries = 3
}

func (ts *FUOTATestSuite) TearDownTest() {
	ts.tx.Rollback()
}

func (ts *FUOTATestSuite) SetupTest() {
	assert := require.New(ts.T())
	var err error
	ts.tx, err = storage.DB().Beginx()
	assert.NoError(err)

	ts.nsClient = nsmock.NewClient()
	networkserver.SetPool(nsmock.NewPool(ts.nsClient))

	ts.NetworkServer = storage.NetworkServer{
		Name:   "test",
		Server: "test:1234",
	}
	assert.NoError(storage.CreateNetworkServer(ts.tx, &ts.NetworkServer))

	ts.Organization = storage.Organization{
		Name: "test-org",
	}
	assert.NoError(storage.CreateOrganization(ts.tx, &ts.Organization))

	ts.ServiceProfile = storage.ServiceProfile{
		Name:            "test-sp",
		OrganizationID:  ts.Organization.ID,
		NetworkServerID: ts.NetworkServer.ID,
	}
	assert.NoError(storage.CreateServiceProfile(ts.tx, &ts.ServiceProfile))
	var spID uuid.UUID
	copy(spID[:], ts.ServiceProfile.ServiceProfile.Id)

	ts.Application = storage.Application{
		Name:             "test-app",
		OrganizationID:   ts.Organization.ID,
		ServiceProfileID: spID,
	}
	assert.NoError(storage.CreateApplication(ts.tx, &ts.Application))

	ts.DeviceProfile = storage.DeviceProfile{
		Name:            "test-dp",
		OrganizationID:  ts.Organization.ID,
		NetworkServerID: ts.NetworkServer.ID,
	}
	assert.NoError(storage.CreateDeviceProfile(ts.tx, &ts.DeviceProfile))
	var dpID uuid.UUID
	copy(dpID[:], ts.DeviceProfile.DeviceProfile.Id)

	ts.Device = storage.Device{
		DevEUI:          lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		ApplicationID:   ts.Application.ID,
		DeviceProfileID: dpID,
		Name:            "test-device",
		Description:     "test device",
	}
	assert.NoError(storage.CreateDevice(ts.tx, &ts.Device))
}

func (ts *FUOTATestSuite) TestFUOTADeploymentMulticastSetupLW10() {
	assert := require.New(ts.T())

	// init
	deviceKeys := storage.DeviceKeys{
		DevEUI:    ts.Device.DevEUI,
		GenAppKey: lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
	}
	assert.NoError(storage.CreateDeviceKeys(ts.tx, &deviceKeys))

	mcg := storage.MulticastGroup{
		Name:  "test-mg",
		MCKey: lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
	}
	copy(mcg.ServiceProfileID[:], ts.ServiceProfile.ServiceProfile.Id)
	assert.NoError(storage.CreateMulticastGroup(ts.tx, &mcg))
	var mcgID uuid.UUID
	copy(mcgID[:], mcg.MulticastGroup.Id)
	mcgReq := <-ts.nsClient.CreateMulticastGroupChan
	ts.nsClient.GetMulticastGroupResponse.MulticastGroup = mcgReq.MulticastGroup

	fd := storage.FUOTADeployment{
		Name:             "test-deployment",
		MulticastGroupID: &mcgID,
		UnicastTimeout:   time.Second,
	}
	assert.NoError(storage.CreateFUOTADeploymentForDevice(ts.tx, &fd, ts.Device.DevEUI))

	// run
	assert.NoError(fuotaDeployments(ts.tx))

	// validate remote multicast setup
	items, err := storage.GetPendingRemoteMulticastSetupItems(ts.tx, 10, 10)
	assert.NoError(err)
	assert.Len(items, 1)

	items[0].CreatedAt = time.Time{}
	items[0].UpdatedAt = time.Time{}

	assert.Equal(storage.RemoteMulticastSetup{
		DevEUI:           ts.Device.DevEUI,
		MulticastGroupID: mcgID,
		MaxMcFCnt:        (1 << 32) - 1,
		McKeyEncrypted:   lorawan.AES128Key{0xe7, 0x12, 0x30, 0xc9, 0x53, 0x24, 0x2, 0x5a, 0x1d, 0xbe, 0xe6, 0x24, 0xcf, 0x67, 0x85, 0xa2},
		State:            storage.RemoteMulticastSetupSetup,
		RetryInterval:    time.Second,
	}, items[0])

	// validate fuota deployment record
	fdUpdated, err := storage.GetFUOTADeployment(ts.tx, fd.ID, false)
	assert.NoError(err)
	assert.Equal(storage.FUOTADeploymentFragmentationSessSetup, fdUpdated.State)
	assert.True(fdUpdated.NextStepAfter.After(time.Now()))
}

func (ts *FUOTATestSuite) TestFUOTADeploymentMulticastSetupLW11() {
	assert := require.New(ts.T())

	// init
	deviceKeys := storage.DeviceKeys{
		DevEUI:    ts.Device.DevEUI,
		AppKey:    lorawan.AES128Key{2, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		GenAppKey: lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
	}
	assert.NoError(storage.CreateDeviceKeys(ts.tx, &deviceKeys))

	mcg := storage.MulticastGroup{
		Name:  "test-mg",
		MCKey: lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
	}
	copy(mcg.ServiceProfileID[:], ts.ServiceProfile.ServiceProfile.Id)
	assert.NoError(storage.CreateMulticastGroup(ts.tx, &mcg))
	var mcgID uuid.UUID
	copy(mcgID[:], mcg.MulticastGroup.Id)
	mcgReq := <-ts.nsClient.CreateMulticastGroupChan
	ts.nsClient.GetMulticastGroupResponse.MulticastGroup = mcgReq.MulticastGroup

	fd := storage.FUOTADeployment{
		Name:             "test-deployment",
		MulticastGroupID: &mcgID,
		UnicastTimeout:   time.Second,
	}
	assert.NoError(storage.CreateFUOTADeploymentForDevice(ts.tx, &fd, ts.Device.DevEUI))

	// run
	assert.NoError(fuotaDeployments(ts.tx))

	// validate remote multicast setup
	items, err := storage.GetPendingRemoteMulticastSetupItems(ts.tx, 10, 10)
	assert.NoError(err)
	assert.Len(items, 1)

	items[0].CreatedAt = time.Time{}
	items[0].UpdatedAt = time.Time{}

	assert.Equal(storage.RemoteMulticastSetup{
		DevEUI:           ts.Device.DevEUI,
		MulticastGroupID: mcgID,
		MaxMcFCnt:        (1 << 32) - 1,
		McKeyEncrypted:   lorawan.AES128Key{0xfb, 0xd1, 0x2a, 0x2e, 0xfa, 0x8d, 0x7f, 0x19, 0x78, 0x83, 0x12, 0x73, 0xac, 0x5b, 0xdb, 0x74},
		State:            storage.RemoteMulticastSetupSetup,
		RetryInterval:    time.Second,
	}, items[0])

	// validate fuota deployment record
	fdUpdated, err := storage.GetFUOTADeployment(ts.tx, fd.ID, false)
	assert.NoError(err)
	assert.Equal(storage.FUOTADeploymentFragmentationSessSetup, fdUpdated.State)
	assert.True(fdUpdated.NextStepAfter.After(time.Now()))
}

func (ts *FUOTATestSuite) TestFUOTADeploymentFragmentationSessionSetup() {
	assert := require.New(ts.T())

	mcg := storage.MulticastGroup{
		Name:  "test-mg",
		MCKey: lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
	}
	copy(mcg.ServiceProfileID[:], ts.ServiceProfile.ServiceProfile.Id)
	assert.NoError(storage.CreateMulticastGroup(ts.tx, &mcg))
	var mcgID uuid.UUID
	copy(mcgID[:], mcg.MulticastGroup.Id)
	mcgReq := <-ts.nsClient.CreateMulticastGroupChan
	ts.nsClient.GetMulticastGroupResponse.MulticastGroup = mcgReq.MulticastGroup

	fd := storage.FUOTADeployment{
		Name:                "test-deployment",
		MulticastGroupID:    &mcgID,
		UnicastTimeout:      time.Second,
		State:               storage.FUOTADeploymentFragmentationSessSetup,
		FragmentationMatrix: 3,
		Descriptor:          [4]byte{1, 2, 3, 4},
		Payload:             []byte{1, 2, 3, 4, 5},
		FragSize:            2,
		Redundancy:          10,
		BlockAckDelay:       4,
	}
	assert.NoError(storage.CreateFUOTADeploymentForDevice(ts.tx, &fd, ts.Device.DevEUI))

	rms := storage.RemoteMulticastSetup{
		DevEUI:           ts.Device.DevEUI,
		MulticastGroupID: mcgID,
		State:            storage.RemoteMulticastSetupSetup,
		StateProvisioned: true,
	}
	assert.NoError(storage.CreateRemoteMulticastSetup(ts.tx, &rms))

	// run
	assert.NoError(fuotaDeployments(ts.tx))

	// validate fragmentation sesssion
	items, err := storage.GetPendingRemoteFragmentationSessions(ts.tx, 10, 10)
	assert.NoError(err)
	assert.Len(items, 1)

	items[0].CreatedAt = time.Time{}
	items[0].UpdatedAt = time.Time{}

	assert.Equal(storage.RemoteFragmentationSession{
		DevEUI:              ts.Device.DevEUI,
		FragIndex:           0,
		MCGroupIDs:          []int{0},
		NbFrag:              13,
		FragSize:            2,
		FragmentationMatrix: 3,
		BlockAckDelay:       4,
		Padding:             1,
		Descriptor:          [4]byte{1, 2, 3, 4},
		State:               storage.RemoteMulticastSetupSetup,
		RetryInterval:       time.Second,
	}, items[0])

	// validate fuota deployment record
	fdUpdated, err := storage.GetFUOTADeployment(ts.tx, fd.ID, false)
	assert.NoError(err)
	assert.Equal(storage.FUOTADeploymentMulticastSessCSetup, fdUpdated.State)
	assert.True(fdUpdated.NextStepAfter.After(time.Now()))
}

func (ts *FUOTATestSuite) TestFUOTADeploymentFragmentationSessionSetupMulticastSetupNotCompleted() {
	assert := require.New(ts.T())

	mcg := storage.MulticastGroup{
		Name:  "test-mg",
		MCKey: lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
	}
	copy(mcg.ServiceProfileID[:], ts.ServiceProfile.ServiceProfile.Id)
	assert.NoError(storage.CreateMulticastGroup(ts.tx, &mcg))
	var mcgID uuid.UUID
	copy(mcgID[:], mcg.MulticastGroup.Id)
	mcgReq := <-ts.nsClient.CreateMulticastGroupChan
	ts.nsClient.GetMulticastGroupResponse.MulticastGroup = mcgReq.MulticastGroup

	fd := storage.FUOTADeployment{
		Name:                "test-deployment",
		MulticastGroupID:    &mcgID,
		UnicastTimeout:      time.Second,
		State:               storage.FUOTADeploymentFragmentationSessSetup,
		FragmentationMatrix: 3,
		Descriptor:          [4]byte{1, 2, 3, 4},
		Payload:             []byte{1, 2, 3, 4, 5},
		FragSize:            2,
		Redundancy:          10,
		BlockAckDelay:       4,
	}
	assert.NoError(storage.CreateFUOTADeploymentForDevice(ts.tx, &fd, ts.Device.DevEUI))

	rms := storage.RemoteMulticastSetup{
		DevEUI:           ts.Device.DevEUI,
		MulticastGroupID: mcgID,
		State:            storage.RemoteMulticastSetupSetup,
		StateProvisioned: false,
	}
	assert.NoError(storage.CreateRemoteMulticastSetup(ts.tx, &rms))

	// run
	assert.NoError(fuotaDeployments(ts.tx))

	// validate fragmentation sesssion
	items, err := storage.GetPendingRemoteFragmentationSessions(ts.tx, 10, 10)
	assert.NoError(err)
	assert.Len(items, 0)
}

func (ts *FUOTATestSuite) TestFUOTADeploymentMulticastSessCSetup() {
	assert := require.New(ts.T())

	mcg := storage.MulticastGroup{
		Name: "test-mg",
		MulticastGroup: ns.MulticastGroup{
			Frequency: 868100000,
			Dr:        5,
		},
	}
	copy(mcg.ServiceProfileID[:], ts.ServiceProfile.ServiceProfile.Id)
	assert.NoError(storage.CreateMulticastGroup(ts.tx, &mcg))
	var mcgID uuid.UUID
	copy(mcgID[:], mcg.MulticastGroup.Id)
	mcgReq := <-ts.nsClient.CreateMulticastGroupChan
	ts.nsClient.GetMulticastGroupResponse.MulticastGroup = mcgReq.MulticastGroup

	fd := storage.FUOTADeployment{
		Name:             "test-deployment",
		MulticastGroupID: &mcgID,
		UnicastTimeout:   time.Second,
		MulticastTimeout: 8,
		State:            storage.FUOTADeploymentMulticastSessCSetup,
	}
	assert.NoError(storage.CreateFUOTADeploymentForDevice(ts.tx, &fd, ts.Device.DevEUI))

	rms := storage.RemoteMulticastSetup{
		DevEUI:           ts.Device.DevEUI,
		MulticastGroupID: mcgID,
		State:            storage.RemoteMulticastSetupSetup,
		StateProvisioned: true,
	}
	assert.NoError(storage.CreateRemoteMulticastSetup(ts.tx, &rms))

	rfs := storage.RemoteFragmentationSession{
		DevEUI:           ts.Device.DevEUI,
		State:            storage.RemoteMulticastSetupSetup,
		StateProvisioned: true,
	}
	assert.NoError(storage.CreateRemoteFragmentationSession(ts.tx, &rfs))

	// run
	assert.NoError(fuotaDeployments(ts.tx))

	// validate class-c sessions
	items, err := storage.GetPendingRemoteMulticastClassCSessions(ts.tx, 10, 10)
	assert.NoError(err)
	assert.Len(items, 1)

	items[0].CreatedAt = time.Time{}
	items[0].UpdatedAt = time.Time{}

	assert.True(items[0].SessionTime.After(time.Now()))
	items[0].SessionTime = time.Time{}

	assert.Equal(storage.RemoteMulticastClassCSession{
		DevEUI:           ts.Device.DevEUI,
		MulticastGroupID: mcgID,
		DLFrequency:      868100000,
		DR:               5,
		SessionTimeOut:   8,
		RetryInterval:    time.Second,
	}, items[0])
}

func (ts *FUOTATestSuite) TestFUOTADeploymentEnqueue() {
	assert := require.New(ts.T())

	mcg := storage.MulticastGroup{
		Name:      "test-mg",
		MCAppSKey: lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		MulticastGroup: ns.MulticastGroup{
			FCnt: 10,
		},
	}
	copy(mcg.ServiceProfileID[:], ts.ServiceProfile.ServiceProfile.Id)
	assert.NoError(storage.CreateMulticastGroup(ts.tx, &mcg))
	var mcgID uuid.UUID
	copy(mcgID[:], mcg.MulticastGroup.Id)
	mcgReq := <-ts.nsClient.CreateMulticastGroupChan
	ts.nsClient.GetMulticastGroupResponse.MulticastGroup = mcgReq.MulticastGroup

	fd := storage.FUOTADeployment{
		Name:             "test-deployment",
		MulticastGroupID: &mcgID,
		Payload:          []byte{1, 2, 3, 4},
		FragSize:         2,
		Redundancy:       1,
		State:            storage.FUOTADeploymentEnqueue,
	}
	assert.NoError(storage.CreateFUOTADeploymentForDevice(ts.tx, &fd, ts.Device.DevEUI))

	// run
	assert.NoError(fuotaDeployments(ts.tx))

	// validate scheduled payloads
	items := []ns.MulticastQueueItem{
		{
			MulticastGroupId: mcgID.Bytes(),
			FrmPayload:       []byte{0xe2, 0xfc, 0x27, 0xb0, 0x1b},
			FCnt:             10,
			FPort:            uint32(fragmentation.DefaultFPort),
		},
		{
			MulticastGroupId: mcgID.Bytes(),
			FrmPayload:       []byte{0x60, 0x1a, 0x2d, 0x1d, 0x37},
			FCnt:             11,
			FPort:            uint32(fragmentation.DefaultFPort),
		},
		{
			MulticastGroupId: mcgID.Bytes(),
			FrmPayload:       []byte{0x76, 0x31, 0x39, 0xac, 0xae},
			FCnt:             12,
			FPort:            uint32(fragmentation.DefaultFPort),
		},
	}

	for _, item := range items {
		req := <-ts.nsClient.EnqueueMulticastQueueItemChan
		assert.Equal(item, *req.MulticastQueueItem)
	}
}

func TestFUOTA(t *testing.T) {
	suite.Run(t, new(FUOTATestSuite))
}
