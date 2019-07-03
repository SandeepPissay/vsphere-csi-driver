# Running e2e Tests

The section outlines how to set the env variable for running e2e test.

## Building e2eTest.conf

```
[Global]
insecure-flag = "true"
hostname = "<VC_IP>"
user = "<USER>"
password = "<PASSWORD>"
port = "443"
datacenters = "<Datacenter_Name>"
```
Please update the `hostname` and `datacenters` as per your testbed configuration.
datacenters should be comma separated if deployed on multi-datacenters

## Setting env variables for All e2e tests
```shell
$ export E2E_TEST_CONF= /path/to/e2eTest.conf
$ export SHARED_VSPHERE_DATASTORE_URL="ds:///vmfs/volumes/5cf05d97-4aac6e02-2940-02003e89d50e/"
$ export NONSHARED_VSPHERE_DATASTORE_URL="ds:///vmfs/volumes/5cf05d98-b2c43515-d903-02003e89d50e/"
$ export STORAGE_POLICY_FOR_SHARED_DATASTORES="vSAN Default Storage Policy"
$ export STORAGE_POLICY_FOR_NONSHARED_DATASTORES="LocalDatastoresPolicy"
```
Please update the values as per your testbed configuration.

## To run full sync test, need do extra following steps

### Setting SSH keys for VC with your local machine to run full sync test

```
1.ssh-keygen -t rsa (ignore if you already have public key in the local env)
2.ssh root@vcip mkdir -p .ssh
3.cat .ssh/id_rsa.pub | ssh root@vcip 'cat >> .ssh/authorized_keys'
4.ssh root@vcip "chmod 700 .ssh; chmod 640 .ssh/authorized_keys"
```

### Setting full sync time var
1. Add `X_CSI_FULL_SYNC_INTERVAL_MINUTES` in csi-driver-deploy.yaml for vsphere-csi-metadata-syncer
2. Setting time interval in the env
```shell
$ export FULL_SYNC_WAIT_TIME=350
$ export USER=root
```
Please update values as per your need.
Make sure env var FULL_SYNC_WAIT_TIME should be at least double of the manifest var in csi-driver-deploy.yaml

## To run a particular test, set it to the string located in “Ginkgo.Describe()” for that test.

To run the Disk Size test (located at https://gitlab.eng.vmware.com/hatchway/vsphere-csi-driver/blob/master/tests/e2e/vsphere_volume_disksize.go)
``` shell
$ export GINKGO_FOCUS=”Volume\sDisk\sSize”
```
Note that specify spaces using “\s”.

