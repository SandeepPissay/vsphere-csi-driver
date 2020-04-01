# Kubernetes vSphere CSI Driver


The [Container Storage Interface (CSI)](https://github.com/container-storage-interface/spec/blob/master/spec.md) is a specification designed to enable persistent storage volume management on Container Orchestrators (COs) such as Kubernetes. This allows storage systems to integrate with containerized workloads running on Kubernetes. Using CSI, third-party storage providers (like VMware) can write and deploy plugins for storage systems in Kubernetes without a need to modify any core Kubernetes code.

CSI allows volume plugins to be deployed (installed) on Kubernetes clusters as extensions. Once a CSI compatible volume driver is deployed on a Kubernetes cluster, users may use the CSI to provision, attach, mount and format the volumes exposed by the CSI driver. In the case of vSphere, the CSI driver is csi.vsphere.vmware.com. 

Please refer documentation for detail.

## Documentation

* [Overview](overview.md)
* [Compatibility Matrix](compatiblity_matrix.md)
* [Supported Feature Matrix](supported_features_matrix.md)
  * [Limits](limits.md)
* Driver Deployment
  * [Prerequisites](driver-deployment/prerequisites.md)
  * [Installation](driver-deployment/installation.md)
  * [Deployment with Zones](driver-deployment/deploying_csi_with_zones.md)
* Features
  * [Volume Provisioning](features/volume_provisioning.md)
  * [Topology Aware Volume Provisioning](features/topology_aware_volume_provisioning.md)
  * [File Share Volumes](features/file_share_volumes.md)
  * [Volume Expansion](features/volume_expansion.md)
* [Upgrade Support Matrix](upgrade_support_matrix.md)
* [Known Issues](known_issues.md)
* [Driver Development](development.md)