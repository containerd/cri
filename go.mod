module github.com/containerd/cri

go 1.13

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/go-winio v0.4.15-0.20190919025122-fc70bd9a86b5
	github.com/Microsoft/hcsshim v0.8.9
	github.com/containerd/cgroups v0.0.0-20200327175542-b44481373989
	github.com/containerd/console v1.0.0 // indirect
	github.com/containerd/containerd v1.4.0-beta.0
	github.com/containerd/continuity v0.0.0-20200413184840-d3ef23f19fbb
	github.com/containerd/fifo v0.0.0-20200410184934-f15a3290365b
	github.com/containerd/go-cni v1.0.0
	github.com/containerd/go-runc v0.0.0-20200220073739-7016d3ce2328 // indirect
	github.com/containerd/imgcrypt v1.0.1
	github.com/containerd/ttrpc v1.0.1 // indirect
	github.com/containerd/typeurl v1.0.1
	github.com/containernetworking/plugins v0.7.6
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/docker v17.12.0-ce-rc1.0.20200310163718-4634ce647cf2+incompatible
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/emicklei/go-restful v2.9.5+incompatible
	github.com/fsnotify/fsnotify v1.4.9
	github.com/gogo/googleapis v1.3.2 // indirect
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.4.2
	github.com/hashicorp/go-multierror v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v1.0.0-rc8.0.20190926000215-3e425f80a8c9
	github.com/opencontainers/runtime-spec v1.0.2
	github.com/opencontainers/selinux v1.5.3-0.20200613095409-bb88c45a3863
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/seccomp/libseccomp-golang v0.9.1 // indirect
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.4.0
	github.com/tchap/go-patricia v2.2.6+incompatible // indirect
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/sys v0.0.0-20200420163511-1957bb5e6d1f
	google.golang.org/genproto v0.0.0-20200224152610-e50cd9704f63 // indirect
	google.golang.org/grpc v1.27.1
	k8s.io/api v0.19.0-beta.2
	k8s.io/apimachinery v0.19.0-beta.2
	k8s.io/apiserver v0.19.0-beta.2
	k8s.io/client-go v0.19.0-beta.2
	k8s.io/component-base v0.19.0-beta.2
	k8s.io/cri-api v0.19.0-beta.2
	k8s.io/klog/v2 v2.2.0
	k8s.io/utils v0.0.0-20200414100711-2df71ebbae66
)
