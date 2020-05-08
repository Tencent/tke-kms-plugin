/*
 * Copyright (C) 2020 THL A29 Limited, a Tencent company.  All rights reserved.
 *
 * TKE KMS Plugin 腾讯云TKE KMS插件 is licensed under the Apache License Version 2.0.
 */

package plugin

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strings"

	kmspb "tke-kms-plugin/v1beta1"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	kms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/kms/v20190118"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"

	"k8s.io/klog"
)

const (
	// Unix Domain Socket
	netProtocol = "unix"

	// Version is the current kms api version
	Version        = "v1beta1"
	runtime        = "Tencent Cloud KMS"
	runtimeVersion = "0.1.0"
)

// KMSServer is the struct containing config and the tke-kms-plugin gRPC server
type KMSServer struct {
	client     *kms.Client
	domain     string
	region     string
	KeyID      string
	unixSocket string
	net.Listener
	*grpc.Server
	lastCreds *common.Credential
	//todo: get role auth
}

// NewKMSServer creates a new KMSServer instance
func NewKMSServer(unixSocket, region, keyID string) (*KMSServer, error) {
	kmsServer := new(KMSServer)
	kmsServer.KeyID = keyID
	kmsServer.unixSocket = unixSocket
	kmsServer.region = region

	cpf := profile.NewClientProfile()
	//cpf.HttpProfile.Endpoint = "kms.tencentcloudapi.com"

	// TODO: role auth
	secretID := strings.TrimSpace(os.Getenv("SECRET_ID"))
	secretKey := strings.TrimSpace(os.Getenv("SECRET_KEY"))
	kmsServer.lastCreds = common.NewCredential(secretID, secretKey)

	client, err := kms.NewClient(kmsServer.lastCreds, kmsServer.region, cpf)
	if err != nil {
		return nil, err
	}
	kmsServer.client = client

	err = validateKeyID(client, keyID, region)
	if err != nil {
		return nil, err
	}

	return kmsServer, nil
}

// TODO: prepare for STS instance role func
func (s *KMSServer) getRoleCreds(seconds int) {

}

func (s *KMSServer) setupRPCServer() error {
	if err := s.Clean(); err != nil {
		return err
	}

	listener, err := net.Listen(netProtocol, s.unixSocket)
	if err != nil {
		return fmt.Errorf("failed to start listener, error: %v", err)
	}
	s.Listener = listener

	server := grpc.NewServer()
	kmspb.RegisterKeyManagementServiceServer(server, s)
	s.Server = server

	go s.Server.Serve(s.Listener)

	klog.Infof("tke-kms-plugin gRPC server started and listening on %s", s.unixSocket)

	return nil
}

// StartRPCServer sets up and starts the tke-kms-plugin gPRC server
func (s *KMSServer) StartRPCServer() (*grpc.Server, error) {
	if err := s.setupRPCServer(); err != nil {
		return nil, err
	}
	return s.Server, nil
}

// Clean removes the unix socket
func (s *KMSServer) Clean() error {
	err := unix.Unlink(s.unixSocket)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete socket file, error: %v", err)
	}
	return nil
}

//Version return the current api version
func (s *KMSServer) Version(ctx context.Context,
	request *kmspb.VersionRequest) (*kmspb.VersionResponse, error) {
	klog.V(4).Info("new request for Version")
	return &kmspb.VersionResponse{
		Version:        Version,
		RuntimeName:    runtime,
		RuntimeVersion: runtimeVersion,
	}, nil
}

//Encrypt execute encryption operation in KMS providers.
func (s *KMSServer) Encrypt(ctx context.Context,
	request *kmspb.EncryptRequest) (*kmspb.EncryptResponse, error) {
	klog.V(4).Info("new encrypt request")
	encReq := kms.NewEncryptRequest()
	params := `{"KeyId":"` + s.KeyID + `","Plaintext":"` +
		base64.StdEncoding.EncodeToString(request.Plain) + `"}`

	err := encReq.FromJsonString(params)
	if err != nil {
		panic(err)
	}
	response, err := s.client.Encrypt(encReq)
	if _, ok := err.(*errors.TencentCloudSDKError); ok {
		klog.Errorf("got API error when doing encryption: %v", err)
	}
	if err != nil {
		panic(err)
	}
	klog.V(4).Info("encrypt successfully")
	return &kmspb.EncryptResponse{Cipher: []byte(*response.Response.CiphertextBlob)}, nil
}

//Decrypt execute decryption operation in KMS providers.
func (s *KMSServer) Decrypt(ctx context.Context,
	request *kmspb.DecryptRequest) (*kmspb.DecryptResponse, error) {
	klog.V(4).Info("new decrypt request")

	decReq := kms.NewDecryptRequest()
	params := `{"CiphertextBlob":"` + string(request.Cipher) + `"}`
	err := decReq.FromJsonString(params)
	if err != nil {
		panic(err)
	}
	response, err := s.client.Decrypt(decReq)
	if _, ok := err.(*errors.TencentCloudSDKError); ok {
		klog.Errorf("got API error when doing decryption: %v", err)
	}
	if err != nil {
		panic(err)
	}
	plaintext, _ := base64.StdEncoding.DecodeString(*response.Response.Plaintext)

	klog.V(4).Info("decrypt successfully")

	return &kmspb.DecryptResponse{Plain: []byte(plaintext)}, nil
}

func validateKeyID(client *kms.Client, keyID, region string) error {
	req := kms.NewDescribeKeyRequest()
	err := req.FromJsonString(`{"keyId":"` + keyID + `"}`)
	if err != nil {
		return err
	}

	resp, err := client.DescribeKey(req)
	if err != nil {
		return err
	}

	// This situation should not happen, since TencentCloudSDKError with
	// code ResourceUnavailable.CmkNotFound will be caugth if key does not exist
	if resp.Response == nil {
		return fmt.Errorf("Key %s does not exist in region %s. Please create it before "+
			"using tke-kms-plugin", keyID, region)
	}

	if *resp.Response.KeyMetadata.KeyState != "Enabled" {
		return fmt.Errorf("Key %s in region %s is not enabled", keyID, region)
	}

	if *resp.Response.KeyMetadata.KeyUsage != "ENCRYPT_DECRYPT" {
		return fmt.Errorf("Key %s in region %s is of type %s, not ENCRYPT_DECRYPT", keyID, region,
			*resp.Response.KeyMetadata.KeyUsage)
	}

	return nil
}
