module k8s.io/ingress-nginx

go 1.20

require (
	github.com/armon/go-proxyproto v0.0.0-20200108142055-f0b8253b1507
	github.com/eapache/channels v1.1.0
	github.com/gogo/protobuf v1.3.2
	github.com/imdario/mergo v0.3.7
	github.com/json-iterator/go v1.1.12
	github.com/kylelemons/godebug v0.0.0-20170820004349-d65d576e9348
	github.com/mitchellh/go-ps v1.0.0
	github.com/mitchellh/hashstructure v1.0.0
	github.com/mitchellh/mapstructure v1.4.1
	github.com/moul/pb v0.0.0-20180404114147-54bdd96e6a52
	github.com/ncabatoff/process-exporter v0.6.0
	github.com/onsi/ginkgo v1.16.4
	github.com/opencontainers/runc v1.1.6
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.14.0
	github.com/prometheus/client_model v0.3.0
	github.com/prometheus/common v0.37.0
	github.com/spf13/cobra v1.6.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.8.1
	github.com/tallclair/mdtoc v0.0.0-20190627191617-4dc3d6f90813
	github.com/zakjan/cert-chain-resolver v0.0.0-20180703112424-6076e1ded272
	golang.org/x/net v0.8.0
	google.golang.org/grpc v1.51.0
	gopkg.in/fsnotify/fsnotify.v1 v1.4.7
	gopkg.in/gavv/httpexpect.v2 v2.0.0
	gopkg.in/go-playground/pool.v3 v3.1.1
	k8s.io/api v0.27.3
	k8s.io/apiextensions-apiserver v0.27.3
	k8s.io/apimachinery v0.27.3
	k8s.io/apiserver v0.27.3
	k8s.io/cli-runtime v0.27.3
	k8s.io/client-go v0.27.3
	k8s.io/code-generator v0.27.3
	k8s.io/component-base v0.27.3
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.90.1
	k8s.io/kubernetes v1.27.3
	pault.ag/go/sniff v0.0.0-20200207005214-cf7e4d167732
	sigs.k8s.io/controller-runtime v0.4.0
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3
)

require (
	github.com/BurntSushi/toml v0.3.1 // indirect
	github.com/ajg/form v1.5.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/coreos/go-systemd/v22 v22.4.0 // indirect
	github.com/cyphar/filepath-securejoin v0.2.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/emicklei/go-restful/v3 v3.9.0 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/fasthttp-contrib/websocket v0.0.0-20160511215533-1f3b11f56072 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/fullsailor/pkcs7 v0.0.0-20190404230743-d7302db945fa // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.1 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/godbus/dbus/v5 v5.0.6 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/gomarkdown/markdown v0.0.0-20190222000725-ee6a7931a1e4 // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/gnostic v0.5.7-v3refs // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/gregjones/httpcache v0.0.0-20180305231024-9cad4c3443a7 // indirect
	github.com/imkira/go-interpol v1.1.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/klauspost/compress v1.4.1 // indirect
	github.com/klauspost/cpuid v1.2.0 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2 // indirect
	github.com/mmarkdown/mmark v2.0.40+incompatible // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/moul/http2curl v1.0.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncabatoff/go-seq v0.0.0-20180805175032-b08ef85ed833 // indirect
	github.com/ncabatoff/procfs v0.0.0-20190407151002-9ced60d7b905 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/gomega v1.27.4 // indirect
	github.com/opencontainers/runtime-spec v1.0.3-0.20220909204839-494a5a6aca78 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/sergi/go-diff v1.1.0 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.2.0 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xlab/treeprint v1.1.0 // indirect
	github.com/yalp/jsonpath v0.0.0-20180802001716-5cc68e5049a0 // indirect
	github.com/yudai/gojsondiff v1.0.0 // indirect
	github.com/yudai/golcs v0.0.0-20170316035057-ecda9a501e82 // indirect
	github.com/yudai/pp v2.0.1+incompatible // indirect
	go.starlark.net v0.0.0-20200306205701-8dd3e2ee1dd5 // indirect
	golang.org/x/mod v0.9.0 // indirect
	golang.org/x/oauth2 v0.0.0-20220223155221-ee480838109b // indirect
	golang.org/x/sys v0.6.0 // indirect
	golang.org/x/term v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/time v0.0.0-20220210224613-90d013bbcef8 // indirect
	golang.org/x/tools v0.7.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220502173005-c8bf987b8c21 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/go-playground/assert.v1 v1.2.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/gengo v0.0.0-20220902162205-c0856e24416d // indirect
	k8s.io/kube-openapi v0.0.0-20230501164219-8b0f38b5fd1f // indirect
	k8s.io/utils v0.0.0-20230209194617-a36077c30491 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/kustomize/api v0.13.2 // indirect
	sigs.k8s.io/kustomize/kyaml v0.14.1 // indirect
	sigs.k8s.io/testing_frameworks v0.1.2 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.27.3
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.27.3
	k8s.io/apimachinery => k8s.io/apimachinery v0.27.3
	k8s.io/apiserver => k8s.io/apiserver v0.27.3
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.27.3
	k8s.io/client-go => k8s.io/client-go v0.27.3
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.27.3
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.27.3
	k8s.io/code-generator => k8s.io/code-generator v0.27.3
	k8s.io/component-base => k8s.io/component-base v0.27.3
	k8s.io/component-helpers => k8s.io/component-helpers v0.27.3
	k8s.io/controller-manager => k8s.io/controller-manager v0.27.3
	k8s.io/cri-api => k8s.io/cri-api v0.27.3
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.27.3
	k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.27.3
	k8s.io/kms => k8s.io/kms v0.27.3
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.27.3
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.27.3
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.27.3
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.27.3
	k8s.io/kubectl => k8s.io/kubectl v0.27.3
	k8s.io/kubelet => k8s.io/kubelet v0.27.3
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.27.3
	k8s.io/metrics => k8s.io/metrics v0.27.3
	k8s.io/mount-utils => k8s.io/mount-utils v0.27.3
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.27.3
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.27.3

)
