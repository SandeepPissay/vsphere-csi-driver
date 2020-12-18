# cnsctl
cnsctl is a fast CLI based tool for storage operations on Cloud Native Storage solution in VMware vSphere. It interacts with VMware vCenter server and Kubernetes to perform various storage operations.

# How to use?
Use "-h" option to know the supported commands flags. Example -
```sh
$ cnsctl -h
A fast CLI based tool to interact with CNS-CSI for various storage control operations.

Usage:
  cnsctl [command]

Available Commands:
  help        Help about any command
  ov          Orphan volume commands

Flags:
  -h, --help      help for cnsctl
      --version   version for cnsctl

Use "cnsctl [command] --help" for more information about a command.
```
The help flag can be used at any command or sub-command level to know the supported flags and sub-commands.

# Supported commands
## Orphan Volume(ov)
Use this command to identify and delete orphan volumes on the vSphere datastore. Currently it supports the following sub-commands:
- ls - list orphan volumes
- rm - remove specific orphan volume(s)
- cleanup - identify orphan volumes and automatically remove them

These sub-commands takes various flags and can work with environment variables. Read the tool help to know the supported flags and environment variables.

### Prerequisites for using ov command
- cnsctl interacts with the Kubernetes cluster and VMware vCenter server to identify the orphan volumes. So, its important to provide the right credentials of the right components to make sure that the tool works as expected.
- cnsctl currently assumes that a datastore is dedicated to a Kubernetes cluster and is not shared across other Kubernetes clusters and/or other applications.
- cnsctl will permanently remove detached and attached orphaned volumes. So use "ls" sub-command to validate if the orphan volumes identified are correct.
- cnsctl requires the vSphere CSI driver to be in maintenance mode(set the replica count of vSphere CSI controller to 0 and wait until the CSI controller pod is terminated). This is to make sure that there are no concurrent volume operations going on while the tool is being used.
- cnsctl assumes that the volumes(aka FCDs) found on the datastore and not used in the Kubernetes cluster as orphans, and will delete them. So, make sure that Kubernetes is the only user of the datastore.
- cnsctl assumes that the VCP to CSI migration is not used in the Kubernetes cluster. And the vSphere volumes(vmdks) are not registered as CNS volumes.
### ls sub-command
Use this sub-command to show the orphan volumes on the datastore(s). This is a read-only operation and does not delete orphan volumes.

Example: List orphan volumes
```sh
$ export CNSCTL_HOST=10.78.166.196
$ export CNSCTL_USER='Administrator@vsphere.local'
$ export CNSCTL_PASSWORD='Admin!23'
$ export CNSCTL_DATACENTER="DC-1"
$ export CNSCTL_KUBECONFIG=~/.kube/config
$ cnsctl -d=vsanDatastore,nfs0-1 ov ls 
Listing all PVs in the Kubernetes cluster...Found 2 PVs in the Kubernetes cluster
Listing FCDs under datastore: vsanDatastore
Found 4 FCDs under datastore: vsanDatastore
Listing FCDs under datastore: nfs0-1
Found 0 FCDs under datastore: nfs0-1
Total volumes: 4
Total orphan volumes: 2
DATASTORE     ORPHAN_VOLUME
vsanDatastore 2c1c1a08-e43a-44d5-ae60-61d6173eaada
vsanDatastore 757dc5cc-2c61-451c-9574-781ab5fa33e6
```

Example: List all volumes(in-use and orphan volumes). This is useful to know all the volumes on the specified datastores.
```sh
$ cnsctl -d=vsanDatastore,nfs0-1 ov ls -a
Listing all PVs in the Kubernetes cluster...Found 2 PVs in the Kubernetes cluster
Listing FCDs under datastore: vsanDatastore
Found 4 FCDs under datastore: vsanDatastore
Listing FCDs under datastore: nfs0-1
Found 0 FCDs under datastore: nfs0-1
Total volumes: 4
Total orphan volumes: 2
DATASTORE     VOLUME_ID                            IS_ORPHAN PV_NAME
vsanDatastore 305e880b-4b71-4750-890c-6b84b7cbe4e0 false     pvc-7fa0df01-6465-4350-bae3-202cb7a9c96c
vsanDatastore 2c1c1a08-e43a-44d5-ae60-61d6173eaada true      
vsanDatastore 79b6245c-20ad-410a-8260-e06c20ff5fa6 false     pvc-c54fd5c6-38ec-49f3-a7a3-c8b6d2d8e016
vsanDatastore 757dc5cc-2c61-451c-9574-781ab5fa33e6 true      
```
### rm sub-command
Use this sub-command to remove orphan volume(s) on a datastore.

Example: Remove 2 orphan volumes
```sh
$ cnsctl -d=vsanDatastore ov rm 2c1c1a08-e43a-44d5-ae60-61d6173eaada 757dc5cc-2c61-451c-9574-781ab5fa33e6
Listing all PVs in the Kubernetes cluster...Found 2 PVs in the Kubernetes cluster
Listing FCDs under datastore: vsanDatastore
Found 4 FCDs under datastore: vsanDatastore
Trying to delete volume: 2c1c1a08-e43a-44d5-ae60-61d6173eaada
Deleted FCD 2c1c1a08-e43a-44d5-ae60-61d6173eaada successfully
Trying to delete volume: 757dc5cc-2c61-451c-9574-781ab5fa33e6
Deleted FCD 757dc5cc-2c61-451c-9574-781ab5fa33e6 successfully
```
rm subcommand does not remove attached orphan volume by default. In order to force remove such attached orphan volumes, use --force flag to the above command. This will detach the orphan volume from the VM and then removes it permanently.

### cleanup sub-command
This sub-command combines the capabilities of ls and rm sub-command and useful when there are many orpahn volumes to be removed. It identifies all the orphan volumes in the specified datastore list and removes them permanently.

Example: Identify and remove orphan volumes
```sh
$ cnsctl -d=vsanDatastore,nfs0-1 ov cleanup
Listing all PVs in the Kubernetes cluster...Found 1 PVs in the Kubernetes cluster
Listing FCDs under datastore: vsanDatastore
Found 3 FCDs under datastore: vsanDatastore
Listing FCDs under datastore: nfs0-1
Found 0 FCDs under datastore: nfs0-1
Found orphan volume: {FcdId:79b6245c-20ad-410a-8260-e06c20ff5fa6 Datastore:vsanDatastore PvName: IsOrphan:true}
Trying to delete volume: 79b6245c-20ad-410a-8260-e06c20ff5fa6
Deleted FCD 79b6245c-20ad-410a-8260-e06c20ff5fa6 successfully
Found orphan volume: {FcdId:d8f5c732-16c6-4337-ba6b-afe11d97a3ac Datastore:vsanDatastore PvName: IsOrphan:true}
Trying to delete volume: d8f5c732-16c6-4337-ba6b-afe11d97a3ac
Deleted FCD d8f5c732-16c6-4337-ba6b-afe11d97a3ac successfully
Cleaned up 2 orphan volumes.
```
cleanup sub-command does not remove orphan volumes that are attached to a VM. To force delete such attached orphan volumes, use --force flag. This will detach the orphan volume before removing them permanently.

## Orphan Volume Attachment(ova)
Use this command to identify and delete orphan volume attachment custom resource(CR) in the Kubernetes cluster. Orphan volume attachment are those volume attachment CRs that are pointing to a persistent volume(PV) that does not exist in the Kubernetes cluster. CSI uses volume attachment CR to attach a persistent volume to a node. It is possible that we will end up with stale orphan volume attachment CRs in few corner cases like manual intervention in deleting PVs.

### ls sub-command
Use this sub-command to show the orphan volume attachment CRs in the Kubernetes cluster. This is a read-only operation and does not delete orphan volume attachment CRs.

Example: List orphan volume attachment CRs
```sh
$ export CNSCTL_KUBECONFIG=/root/.kube/config
$ cnsctl ova ls
Found 0 PVs in the Kubernetes cluster

ORPHAN_VOLUME_ATTACHMENT_NAME                                        PV_NAME                                  ATTACH_NODE IS_ATTACHED
csi-05b25499532888adb9ba4d8ef1f8574affeaf38d9201875d92dd217d6e7a3295 pvc-3736e2a1-babc-4301-8882-0b65d1664bec k8s-node1   true

----------------------- Summary ------------------------------
Total volume attachment CRs found: 1
Total orphan volume attachment CRs found: 1
```
Use -a flag to this sub-command to list orphan and in-use volume attachment CRs.

### cleanup sub-command
Use cleanup sub-command to remove the orphan volume attachment CRs. Note that this removes all the finalizers on the orphan volume attachment CR before deleting the CR.
```sh
$ cnsctl ova cleanup
Found 0 PVs in the Kubernetes cluster
(1/1) Trying to delete VolumeAttachment: csi-05b25499532888adb9ba4d8ef1f8574affeaf38d9201875d92dd217d6e7a3295 for PV pvc-3736e2a1-babc-4301-8882-0b65d1664bec
VolumeAttachment csi-05b25499532888adb9ba4d8ef1f8574affeaf38d9201875d92dd217d6e7a3295 for PV pvc-3736e2a1-babc-4301-8882-0b65d1664bec got deleted after removing finalizers.

----------------------- Summary ------------------------------
Total volume attachment CRs found: 1
Total orphan volume attachment CRs found: 1
Total orphan volume attachment deleted: 1
```
