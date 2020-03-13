package willy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/chirpstack-geolocation-server/internal/config"
	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/geo"
	"github.com/brocaar/lorawan"
)

// Backend exposes the Willy geolocation- methods.
type Backend struct {
	requestTimeout  time.Duration
}

// NewBackend creates a new backend.
func NewBackend(c config.Config) (geo.GeolocationServerServiceServer, error) {
	return &Backend{
		requestTimeout:  c.GeoServer.Backend.Willy.RequestTimeout,
	}, nil
}

// ResolveTDOA resolves the location based on TDOA.
func (b *Backend) ResolveTDOA(ctx context.Context, req *geo.ResolveTDOARequest) (*geo.ResolveTDOAResponse, error) {
	willyReq, err := resolveTDOARequestToWillyRequest(req)
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}

	tdoaResp, err := b.resolveTDOA(ctx, willyReq)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, "geolocation error: %s", err)
	}

	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	if len(tdoaResp.Warnings) != 0 {
		log.WithFields(log.Fields{
			"dev_eui":  devEUI,
			"warnings": tdoaResp.Warnings,
		}).Warning("backend/willy: backend returned warnings")
	}

	if len(tdoaResp.Errors) != 0 {
		log.WithFields(log.Fields{
			"dev_eui": devEUI,
			"errors":  tdoaResp.Errors,
		}).Error("backend/willy: backend returned errors")

		return nil, grpc.Errorf(codes.Internal, "backend returned errors: %v", tdoaResp.Errors)
	}

	return &geo.ResolveTDOAResponse{
		Result: &geo.ResolveResult{
			Location: &common.Location{
				Source:    common.LocationSource_GEO_RESOLVER,
				Accuracy:  uint32(tdoaResp.Result.Accuracy),
				Latitude:  tdoaResp.Result.Latitude,
				Longitude: tdoaResp.Result.Longitude,
				Altitude:  tdoaResp.Result.Altitude,
			},
		},
	}, nil

}

func (b *Backend) ResolveMultiFrameTDOA(ctx context.Context, req *geo.ResolveMultiFrameTDOARequest) (*geo.ResolveMultiFrameTDOAResponse, error) {
	willyReq, err := resolveMutiFrameTDOARequestToWillyRequest(req)
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}

	tdoaResp, err := b.resolveTDOAMultiFrame(ctx, willyReq)
	if err != nil {
		return nil, grpc.Errorf(codes.Unknown, "geolocation error: %s", err)
	}

	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	if len(tdoaResp.Warnings) != 0 {
		log.WithFields(log.Fields{
			"dev_eui":  devEUI,
			"warnings": tdoaResp.Warnings,
		}).Warning("backend/willy: backend returned warnings")
	}

	if len(tdoaResp.Errors) != 0 {
		log.WithFields(log.Fields{
			"dev_eui": devEUI,
			"errors":  tdoaResp.Errors,
		}).Error("backend/willy: backend returned errors")

		return nil, grpc.Errorf(codes.Internal, "backend returned errors: %v", tdoaResp.Errors)
	}

	return &geo.ResolveMultiFrameTDOAResponse{
		Result: &geo.ResolveResult{
			Location: &common.Location{
				Source:    common.LocationSource_GEO_RESOLVER,
				Accuracy:  uint32(tdoaResp.Result.Accuracy),
				Latitude:  tdoaResp.Result.Latitude,
				Longitude: tdoaResp.Result.Longitude,
				Altitude:  tdoaResp.Result.Altitude,
			},
		},
	}, nil
}

func (b *Backend) resolveTDOA(ctx context.Context, tdoaReq tdoaRequest) (response, error) {
	d := willyAPIDuration("v2_tdoa")
	start := time.Now()
	resp, err := b.willyAPIRequest(ctx, tdoaEndpoint, tdoaReq)
	d.Observe(float64(time.Since(start)) / float64(time.Second))
	return resp, err
}

func (b *Backend) resolveTDOAMultiFrame(ctx context.Context, tdoaMultiFrameReq tdoaMultiFrameRequest) (response, error) {
	d := willyAPIDuration("v2_tdoa_multiframe")
	start := time.Now()
	resp, err := b.willyAPIRequest(ctx, tdoaMultiFrameEndpoint, tdoaMultiFrameReq)
	d.Observe(float64(time.Since(start)) / float64(time.Second))
	return resp, err
}

func (b *Backend) willyAPIRequest(ctx context.Context, endpoint string, v interface{}) (response, error) {
	var resolveResp response

	bb, err := json.Marshal(v)
	if err != nil {
		return resolveResp, errors.Wrap(err, "marshal request error")
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(bb))
	if err != nil {
		return resolveResp, errors.Wrap(err, "new request error")
	}

	req.Header.Set("Content-Type", "application/json")

	reqCTX, cancel := context.WithTimeout(ctx, b.requestTimeout*1000000)
	defer cancel()

	req = req.WithContext(reqCTX)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return resolveResp, errors.Wrap(err, "http request error")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bb, _ := ioutil.ReadAll(resp.Body)
		return resolveResp, fmt.Errorf("expected 200, got: %d (%s)", resp.StatusCode, string(bb))
	}

	if err = json.NewDecoder(resp.Body).Decode(&resolveResp); err != nil {
		return resolveResp, errors.Wrap(err, "unmarshal response error")
	}

	return resolveResp, nil
}
