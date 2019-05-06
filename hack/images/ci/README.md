# Continuous integration

The image `vsphere-csi-driver/ci` is used to build, test, and deploy the CSI providers.

## Mapping up-to-date sources

When running on CI/CD systems the jobs map the current sources into the CI container. That may be simulated locally by running the examples from a directory containing the desired sources and providing the `docker run` command with the following flags:

* `-v "$(pwd)":/go/src/sigs.k8s.io/vsphere-csi-driver`

## Docker-in-Docker

Several of the jobs require Docker-in-Docker. To mimic that locally there are two options:

1. [Provide the host's Docker to the container](#provide-the-hosts-docker-to-the-container)
2. [Run the Docker server inside the container](#run-the-docker-server-inside-the-container)

### Provide the host's Docker to the container

While Prow jobs [run the Docker server inside the container](#run-the-docker-server-inside-the-container), this option provides a low-cost (memory, disk) solution for testing locally. This option is enabled by running the examples from a directory containing the desired sources and providing the `docker run` command with the following flags:

* `-v /var/run/docker.sock:/var/run/docker.sock`
* `-e "PROJECT_ROOT=$(pwd)"`
* `-v "$(pwd)":/go/src/sigs.k8s.io/vsphere-csi-driver`

Please note that this option is only available when using a local copy of the sources. This is because all of the paths known to Docker will be of the local host system, not from the container. That's also why it's necessary to provide the `PROJECT_ROOT` environment variable -- it indicates to certain recipes the location of specific files or directories relative to the local sources on the host system.

### Run the Docker server inside the container
This is option that Prow jobs utilize and is also the method illustrated by the examples below. Please keep in mind that using this option locally requires a large amount of memory and disk space available to Docker:

| Type | Minimum Requirement |
|------|---------------------|
| Memory | 8GiB |
| Disk | 200GiB |

For Windows and macOS systems this means adjusting the size of the Docker VM disk and the amount of memory the Docker VM is allowed to use.

Resources notwithstanding, running the Docker server inside the container also requires providing the `docker run` command with the following flags:

* `--privileged`

## Check the sources

To check the sources run the following command:

```shell
$ docker run -it --rm \
  -e "ARTIFACTS=/out" -v "$(pwd)":/out \
  -v "$(pwd)":/go/src/sigs.k8s.io/vsphere-csi-driver \
  vsphere-csi-driver/ci \
  make check
```

The above command will create the following files in the working directory:

* `junit_check.xml`

## Build the CSI and Syncer binaries

The CI image is built with Go module and build caches from a recent build of the project's `master` branch. Therefore the CI image can be used to build the CSI and Syncer binaries in a matter of seconds:

```shell
$ docker run -it --rm \
  -e "BIN_OUT=/out" -v "$(pwd)":/out \
  -v "$(pwd)":/go/src/sigs.k8s.io/vsphere-csi-driver \
  vsphere-csi-driver/ci \
  make build
```

The above command will create the following files in the working directory:

* `vsphere-csi.linux_amd64`
* `syncer.linux_amd64`

## Execute the unit tests

```shell
$ docker run -it --rm \
  -v "$(pwd)":/go/src/sigs.k8s.io/vsphere-csi-driver \
  vsphere-csi-driver/ci \
  make unit-test
```
