# Kubernetes KMS provider plugin, backed by Tencent Cloud KMS service

## Overview

[Kubernetes KMS provider](https://kubernetes.io/docs/tasks/administer-cluster/kms-provider/) plugin for Tencent Cloud - Enable encryption/decryption for Kubernetes secrets by Tencent Cloud Key Management Service

## Prerequisites

- Kubernetes cluster of version 1.10.0 or later(e.g. TKE standalone clusters of version v1.10.5+)
- Etcd v3 or later

## Build image

1. Clone this repo and cd to `tke-kms-plugin`
2. Build and push image
```bash
#such as ccr.ccs.tencentyun.com/<your-project>
REGISTRY=<your-docker-registry-prefix>
VERSION=1.0.0
REGISTRY=$REGISTRY VERSION=$VERSION make image

docker push $REGISTRY/tke-kms-plugin:$VERSION
```

## Configuring the KMS provider
### Deploy KMS provider plugin
1. Create a new key of type `ENCRYPT_DECRYPT`(对称加解密) at [KMS Console](https://console.cloud.tencent.com/kms2)
2. Create a new CAM Secret at [CAM console](https://console.cloud.tencent.com/cam) or use an existing one(The secret should at least be allowed do the following actions : `"kms:Decrypt", "kms:DescribeKey", "kms:Encrypt", "kms:ReEncrypt"`).
3. Replace following placeholders in [tke-kms-plugin.yaml](deployment/tke-kms-plugin.yaml):
  - `{{REGION}}`: the region of KMS service, valid values: `ap-beijing`, `ap-guangzhou`, `ap-shanghai`
  - `{{KEY_ID}}`: the id (in UUID format) of the KMS key you just created
  - `{{SECRET_ID}}` and `{{SECRET_KEY}}`: the CAM Secret ID and Key you just created
  ```bash
  REGION=<region>
  KEY_ID=<your-key-id>
  SECRET_ID=<your-secret-id>
  SECRET_KEY=<your-secret-key>

  sed "s/{{REGION}}/$REGION/g; s/{{KEY_ID}}/$KEY_ID/g; s/{{SECRET_ID}}/$SECRET_ID/g; s/{{SECRET_KEY}}/$SECRET_KEY/g" deployment/tke-kms-plugin.yaml > ds-tke-kms-plugin.yaml

  ```
4. (Optional) If you prefer to use your own docker image, replace `images: ccr.ccs.tencentyun.com/tke-plugin/tke-kms-plugin:1.0.0` with your own image.
5. Create the tke-kms-plugin daemonset: `kubectl apply -f ds-tke-kms-plugin.yaml`
6. Ensure all tke-kms-plugin pods are running: `kubectl get po -n kube-system -l name=tke-kms-plugin`

### Configuring kube-apiserver
**On all masters:**

1. Create `/etc/kubernetes/encryption-provider-config.yaml` with following content:

  **Note**: For K8s v1.10~v1.13, you can specify the `timeout` parameter, although it is not supported.

 + for K8s v1.13+
```yaml
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
      - secrets
    providers:
      - kms:
          name: tke-kms-plugin
          timeout: 3s
          cachesize: 1000
          endpoint: unix:///var/run/tke-kms-plugin/server.sock
      - identity: {}
```

  + for K8s v1.10~v1.12
```yaml
apiVersion: v1
kind: EncryptionConfig
resources:
  - resources:
      - secrets
    providers:
      - kms:
          name: tke-kms-plugin
          timeout: 3s
          cachesize: 1000
          endpoint: unix:///var/run/tke-kms-plugin/server.sock
      - identity: {}

```

2. Edit `/etc/kubernetes/manifests/kube-apiserver.yaml` (for TKE standalone clusters. And for TKE v1.10.5, you need to move `kube-apiserver.yaml` out of `/etc/kubernetes/manifests` directory, edit and move it back)：

+ Add the following flag to `args` array:
    - for K8s v1.13+ : `--encryption-provider-config=/etc/kubernetes/encryption-provider-config.yaml`
    - for K8s v1.10~v1.12: `--experimental-encryption-provider-config=/etc/kubernetes/encryption-provider-config.yaml` 

+ Add volume directives for `/var/run/tke-kms-plugin/server.sock`:

```yaml
...
    volumeMounts:
    - mountPath: /var/run/tke-kms-plugin
      name: tke-kms-plugin-dir
...
  volumes:
  - hostPath:
      path: /var/run/tke-kms-plugin
    name: tke-kms-plugin-dir
...    
```

kube-apiserver will restart after you finish editing and save the file `/etc/kubernetes/manifests/kube-apiserver.yaml`.

## Verifying

1 Create a new secret

```
kubectl create secret generic secret1 -n default --from-literal=mykey=mydata
```

2 Verify the secret is correctly decrypted:

```
kubectl get secret secret1 -o=jsonpath='{.data.mykey}' | base64 -d
```
The output should be `mydata`, the same as the value we used to create the secret.

## Reference

For more infomation about Kubernetes KMS provider, please refer to [Using a KMS provider for data encryption](https://kubernetes.io/docs/tasks/administer-cluster/kms-provider/)