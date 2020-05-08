/*
 * Copyright (C) 2020 THL A29 Limited, a Tencent company.  All rights reserved.
 *
 * TKE KMS Plugin 腾讯云TKE KMS插件 is licensed under the Apache License Version 2.0.
 */

package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"tke-kms-plugin/plugin"
	kmspb "tke-kms-plugin/v1beta1"

	"google.golang.org/grpc"

	"k8s.io/klog"
)

const (
	healthCheckCmdName = "health-check"
)

var (
	region, unixSocket, keyID string
)

var healthCheckCmd = flag.NewFlagSet(healthCheckCmdName, flag.ExitOnError)

func init() {
	// start kms plugin server: tke-kms-plugin --listen-unix-socket=<unix-socket>
	flag.StringVar(&region, "region", "ap-beijing",
		"the region of Tencent KMS service(supported regions: ap-beijing, ap-guangzhou, "+
			"ap-shanghai)")
	flag.StringVar(&unixSocket, "unix-socket", "/var/run/tke-kms-plugin/server.sock",
		"the unix socket that tke kms plugin listens on")
	flag.StringVar(&keyID, "key-id", "",
		"the Tencent KMS key ID used for tke kms plugin to do encrption and decryption."+
			" The key must be created before use")

	// check kms plugin health: tke-kms-plugin health-check --listen-unix-socket=<unix-socket>
	healthCheckCmd.StringVar(&unixSocket, "unix-socket", "/var/run/tke-kms-plugin/server.sock",
		"the socket used to connection with kmsServer")
}

func main() {
	// health-check for liveness probe
	if len(os.Args) > 1 && os.Args[1] == healthCheckCmdName {
		healthCheckCmd.Parse(os.Args[2:])

		err := doHealthCheck(unixSocket)
		if err != nil {
			klog.Errorf("health-check failed: %v", err)
			os.Exit(1)
		}
		return
	}

	klog.InitFlags(nil)
	flag.Parse()

	validateFlags()

	kmsServer, err := plugin.NewKMSServer(unixSocket, region, keyID)
	if err != nil {
		klog.Fatalf("unable to create KMS server: %v", err)
	}

	s, err := kmsServer.StartRPCServer()
	if err != nil {
		klog.Fatalf("unable to start KMS RPC server: %v", err)
	}
	defer func() {
		s.GracefulStop()
		klog.Info("KMS RPC server gracefully stopped")
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	select {
	case s := <-signalCh:
		klog.Infof("got signal %s, exiting...", s)
	}
}

func doHealthCheck(unixSocket string) error {
	Version := "v1beta1"

	connection, err := dialUnix(unixSocket)
	if err != nil {
		return fmt.Errorf("unable to connect with grpc socket: %v", err)
	}

	client := kmspb.NewKeyManagementServiceClient(connection)

	request := &kmspb.VersionRequest{Version: Version}
	_, err = client.Version(context.Background(), request)
	if err != nil {
		return fmt.Errorf("unable to get version from tke-kms-plgin grpc endpoing %s: %v",
			unixSocket, err)
	}

	encryptRequest := kmspb.EncryptRequest{Version: Version, Plain: []byte("test")}
	encryptResponse, err := client.Encrypt(context.Background(), &encryptRequest)
	if err != nil {
		return fmt.Errorf("unable to do encryption with remote KMS provider: %v", err)
	}

	decryptRequest := kmspb.DecryptRequest{Version: Version, Cipher: encryptResponse.Cipher}
	_, err = client.Decrypt(context.Background(), &decryptRequest)
	if err != nil {
		return fmt.Errorf("unable to do decryption with remote KMS provider: %v", err)
	}

	return nil
}

func dialUnix(unixSocketPath string) (*grpc.ClientConn, error) {
	protocol, addr := "unix", unixSocketPath
	dialer := func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout(protocol, addr, timeout)
	}
	return grpc.Dial(addr, grpc.WithInsecure(), grpc.WithDialer(dialer))
}

func validateFlags() {
	if strings.TrimSpace(keyID) == "" {
		klog.Fatal("--key-id is not specified")
	}

	secretID := strings.TrimSpace(os.Getenv("SECRET_ID"))
	secretKey := strings.TrimSpace(os.Getenv("SECRET_KEY"))
	if secretID == "" || secretKey == "" {
		klog.Fatal("SECRET_ID and SECRET_KEY must be set in env")
	}
}
