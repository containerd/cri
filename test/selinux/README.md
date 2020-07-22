# SELinux Testing with Vagrant

First, [install Vagrant](https://www.vagrantup.com/docs/installation).

Second, this supplied `Vagrantfile` attempts to share your local (single entry) $GOPATH with the guest VM.
If you are using the VirtualBox provider, the likely default, you will also need to install the guest plugin:

```shell script
vagrant plugin install vagrant-vbguest
```

Connect to the VM:

```shell script
vagrant up
vagrant ssh
```

Run the tests:

```shell script
cd $GOPATH/src/github.com/containerd/cri
# runc and cri-tools are already installed
#
# this cni installation does not conflict with the cni plugins installed via yum
# which is good because containerd/cri is still using v0.7.x and testing with a cidr
# that the 0.8.x plugins refuse to use
./hack/install/install-cni.sh
./hack/install/install-cni-config.sh
./hack/install/install-containerd.sh
# selinux enforcing
sudo setenforce 1
make clean test-cri
#make test-integration
```
