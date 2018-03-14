# Install CRI-Containerd with Release Tarball
This document provides the steps to install `cri-containerd` and its dependencies with the release tarball, and bring up a Kubernetes cluster using kubeadm.

These steps have been verified on Ubuntu 16.04. For other OS distributions, the steps may differ. Please feel free to file issues or PRs if you encounter any problems on other OS distributions.

*Note: You need to run the following steps on each node you are planning to use in your Kubernetes cluster.*
## Release Tarball
For each release, we'll publish a release tarball. The release tarball contains all required binaries and files for `cri-containerd`.
### Content
<!-- TODO(random-liu): Update the release tarball information -->
As shown below, the release tarball contains:
1) `containerd`, `containerd-shim`, `containerd-stress`, `containerd-release`, `ctr`: binaries for containerd.
2) `runc`: runc binary.
3) `crictl`: command line tools for CRI container runtime.
4) `containerd.service`: Systemd unit for containerd.
5) `/opt/cri-containerd/cluster/`: scripts for `kube-up.sh`.
```console
$ tar -tf cri-containerd-1.0.0-beta.0.linux-amd64.tar.gz
./
./opt/
./opt/cri-containerd/
./opt/cri-containerd/cluster/
./opt/cri-containerd/cluster/gce/
./opt/cri-containerd/cluster/gce/cloud-init/
./opt/cri-containerd/cluster/gce/cloud-init/node.yaml
./opt/cri-containerd/cluster/gce/cloud-init/master.yaml
./opt/cri-containerd/cluster/gce/configure.sh
./opt/cri-containerd/cluster/health-monitor.sh
./opt/cri-containerd/cluster/env
./usr/
./usr/local/
./usr/local/sbin/
./usr/local/sbin/runc
./usr/local/bin/
./usr/local/bin/crictl
./usr/local/bin/containerd
./usr/local/bin/containerd-stress
./usr/local/bin/critest
./usr/local/bin/containerd-release
./usr/local/bin/containerd-shim
./usr/local/bin/ctr
./etc/
./etc/systemd/
./etc/systemd/system/
./etc/systemd/system/containerd.service
./etc/crictl.yaml
```
### Binary Information
Information about the binaries in the release tarball:

|           Binary Name          |      Support       |   OS  | Architecture |
|:------------------------------:|:------------------:|:-----:|:------------:|
|            containerd          | seccomp, apparmor, | linux |     amd64    |
|                                |   overlay, btrfs   |       |              |
|          containerd-shim       |   overlay, btrfs   | linux |     amd64    |
|               runc             | seccomp, apparmor  | linux |     amd64    |


If you have other requirements for the binaries, e.g. selinux support, another architecture support etc., you need to build the binaries yourself following [the instructions](../README.md#getting-started-for-developers).

### Download

The release tarball could be downloaded from either of the following sources:
1. Release on github (see [here](https://github.com/containerd/cri/releases));
2. Release GCS bucket https://storage.googleapis.com/cri-containerd-release/.

## Step 0: Install Dependent Libraries
Install required libraries for seccomp and libapparmor.
```bash
sudo apt-get update
sudo apt-get install libseccomp2
sudo apt-get install libapparmor
```
Note that:
1) If you are using Ubuntu <=Trusty or Debian <=jessie, a backported version of `libseccomp2` is needed. (See the [trusty-backports](https://packages.ubuntu.com/trusty-backports/libseccomp2) and [jessie-backports](https://packages.debian.org/jessie-backports/libseccomp2)).
2) If your OS distro doesn't support AppArmor, please skip installing `libapparmor`, and AppArmor will be disabled.
## Step 1: Download CRI-Containerd Release Tarball
Download release tarball for the cri-containerd version you want to install from sources listed [above](#download).
```bash
wget https://storage.googleapis.com/cri-containerd-release/cri-containerd-${VERSION}.linux-amd64.tar.gz
```
Validate checksum of the release tarball (note that checksum is added since v1.0.0-beta.0):
```bash
sha256sum cri-containerd-${VERSION}.linux-amd64.tar.gz
curl https://storage.googleapis.com/cri-containerd-release/cri-containerd-${VERSION}.linux-amd64.tar.gz.sha256
# Compare to make sure the 2 checksums are the same.
```
## Step 2: Install CRI-Containerd
If you are using systemd, just simply unpack the tarball to the root directory:
<!-- TODO(random-liu): Remove cni directory operations after we deprecate v1.0.0-beta.1 -->
```bash
sudo tar -C / -xzf cri-containerd-${VERSION}.linux-amd64.tar.gz
sudo mkdir -p /opt/cni/bin/
sudo mkdir -p /etc/cni/net.d
sudo systemctl start containerd
```
If you are not using systemd, please unpack all binaries into a directory in your `PATH`, and start `containerd` as monitored long running services with the service manager you are using e.g. `supervisord`, `upstart` etc.
## Step 3: Install Kubeadm, Kubelet and Kubectl
Follow [the instructions](https://kubernetes.io/docs/setup/independent/install-kubeadm/) to install kubeadm, kubelet and kubectl.
## Step 4: Create Systemd Drop-In for CRI-Containerd
Create the systemd drop-in file `/etc/systemd/system/kubelet.service.d/0-cri-containerd.conf`:
```
[Service]                                                 
Environment="KUBELET_EXTRA_ARGS=--container-runtime=remote --runtime-request-timeout=15m --container-runtime-endpoint=/run/containerd/containerd.sock"
```
And reload systemd configuration:
```bash
systemctl daemon-reload
```
## Bring Up the Cluster
Now you should have properly installed all required binaries and dependencies on each of your node.

The next step is to use kubeadm to bring up the Kubernetes cluster. It is the same with [the ansible installer](../contrib/ansible). Please follow the steps 2-4 [here](../contrib/ansible/README.md#step-2).
