package external

import (
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/jmoiron/sqlx"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/lora-app-server/api"
	"github.com/brocaar/lora-app-server/internal/api/external/auth"
	"github.com/brocaar/lora-app-server/internal/api/helpers"
	"github.com/brocaar/lora-app-server/internal/backend/networkserver"
	"github.com/brocaar/lora-app-server/internal/storage"
	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

// FUOTADeploymentAPI exports the FUOTA deployment related functions.
type FUOTADeploymentAPI struct {
	validator auth.Validator
}

// NewFUOTADeploymentAPI creates a new FUOTADeploymentAPI.
func NewFUOTADeploymentAPI(validator auth.Validator) *FUOTADeploymentAPI {
	return &FUOTADeploymentAPI{
		validator: validator,
	}
}

// CreateForDevEUI creates a deployment for the given DevEUI.
func (f *FUOTADeploymentAPI) CreateForDevEUI(ctx context.Context, req *api.CreateFUOTADeploymentForDevEUIRequest) (*api.CreateFUOTADeploymentForDevEUIResponse, error) {
	if req.FuotaDeployment == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "fuota_deployment must not be nil")
	}

	var devEUI lorawan.EUI64
	if err := devEUI.UnmarshalText([]byte(req.DevEui)); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}

	if err := f.validator.Validate(ctx,
		auth.ValidateFUOTADeploymentsAccess(auth.Create, 0, devEUI)); err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	n, err := storage.GetNetworkServerForDevEUI(storage.DB(), devEUI)
	if err != nil {
		return nil, helpers.ErrToRPCError(err)
	}

	nsClient, err := networkserver.GetPool().Get(n.Server, []byte(n.CACert), []byte(n.TLSCert), []byte(n.TLSKey))
	if err != nil {
		return nil, helpers.ErrToRPCError(err)
	}

	versionResp, err := nsClient.GetVersion(ctx, &empty.Empty{})
	if err != nil {
		return nil, helpers.ErrToRPCError(err)
	}

	var b band.Band

	switch versionResp.Region {
	case common.Region_EU868:
		b, err = band.GetConfig(band.EU_863_870, false, lorawan.DwellTimeNoLimit)
		if err != nil {
			return nil, helpers.ErrToRPCError(err)
		}
	default:
		return nil, grpc.Errorf(codes.Internal, "region %s is not implemented", versionResp.Region)
	}

	maxPLSize, err := b.GetMaxPayloadSizeForDataRateIndex("", "", int(req.FuotaDeployment.Dr))
	if err != nil {
		return nil, helpers.ErrToRPCError(err)
	}

	fd := storage.FUOTADeployment{
		Name:             req.FuotaDeployment.Name,
		DR:               int(req.FuotaDeployment.Dr),
		Frequency:        int(req.FuotaDeployment.Frequency),
		Payload:          req.FuotaDeployment.Payload,
		FragSize:         maxPLSize.N - 3,
		Redundancy:       int(req.FuotaDeployment.Redundancy),
		MulticastTimeout: int(req.FuotaDeployment.MulticastTimeout),
	}

	switch req.FuotaDeployment.GroupType {
	case api.MulticastGroupType_CLASS_C:
		fd.GroupType = storage.FUOTADeploymentGroupTypeC
	default:
		return nil, grpc.Errorf(codes.InvalidArgument, "group_type %s is not supported", req.FuotaDeployment.GroupType)
	}

	fd.UnicastTimeout, err = ptypes.Duration(req.FuotaDeployment.UnicastTimeout)
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "unicast_timeout: %s", err)
	}

	err = storage.Transaction(func(db sqlx.Ext) error {
		return storage.CreateFUOTADeploymentForDevice(db, &fd, devEUI)
	})
	if err != nil {
		return nil, helpers.ErrToRPCError(err)
	}

	return &api.CreateFUOTADeploymentForDevEUIResponse{
		Id: fd.ID.String(),
	}, nil
}

// Get returns the fuota deployment for the given id.
func (f *FUOTADeploymentAPI) Get(ctx context.Context, req *api.GetFUOTADeploymentRequest) (*api.GetFUOTADeploymentResponse, error) {
	panic("not implemented")
}

// ListDevices lists the devices (and status) for the given fuota deployment ID.
func (f *FUOTADeploymentAPI) ListDevices(ctx context.Context, req *api.ListFUOTADeploymentDevicesRequest) (*api.ListFUOTADeploymentDevicesResponse, error) {
	panic("not implemented")
}
