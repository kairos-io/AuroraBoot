module github.com/kairos-io/AuroraBoot

go 1.23.6

toolchain go1.24.0

// https://github.com/golang/go/blob/583d750fa119d504686c737be6a898994b674b69/src/crypto/x509/parser.go#L1014-L1018
// For keys with negative serial number:
godebug x509negativeserial=1

require (
	github.com/cavaliergopher/grab/v3 v3.0.1
	github.com/containerd/containerd v1.7.26
	github.com/diskfs/go-diskfs v1.4.2
	github.com/foxboron/go-uefi v0.0.0-20241219185318-19dc140271bf
	github.com/foxboron/sbctl v0.0.0-20240526163235-64e649b31c8e
	github.com/gofrs/uuid v4.4.0+incompatible
	github.com/google/go-containerregistry v0.20.3
	github.com/hashicorp/go-multierror v1.1.1
	github.com/kairos-io/go-ukify v0.2.5
	github.com/kairos-io/kairos-agent/v2 v2.16.3
	github.com/kairos-io/kairos-sdk v0.7.3
	github.com/kairos-io/netboot v0.0.0-20241104101831-1454e04fdb07
	github.com/klauspost/compress v1.17.11
	github.com/mudler/go-processmanager v0.0.0-20240820160718-8b802d3ecf82
	github.com/mudler/yip v1.15.0
	github.com/onsi/ginkgo/v2 v2.22.2
	github.com/onsi/gomega v1.36.2
	github.com/otiai10/copy v1.14.1
	github.com/sanity-io/litter v1.5.8
	github.com/spectrocloud-labs/herd v0.4.2
	github.com/spectrocloud/peg v0.0.0-20240405075800-c5da7125e30f
	github.com/spf13/viper v1.19.0
	github.com/twpayne/go-vfs/v5 v5.0.4
	github.com/u-root/u-root v0.14.0
	github.com/urfave/cli/v2 v2.27.5
	golang.org/x/exp v0.0.0-20250106191152-7588d65b2ba8
	golang.org/x/mod v0.23.0
	golang.org/x/sys v0.30.0
	gopkg.in/yaml.v1 v1.0.0-20140924161607-9f9df34309c0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	atomicgo.dev/cursor v0.2.0 // indirect
	atomicgo.dev/keyboard v0.2.9 // indirect
	atomicgo.dev/schedule v0.1.0 // indirect
	dario.cat/mergo v1.0.1 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.3.1 // indirect
	github.com/Masterminds/sprig/v3 v3.3.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/Microsoft/hcsshim v0.12.9 // indirect
	github.com/ProtonMail/go-crypto v1.1.5 // indirect
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/avast/retry-go v3.0.0+incompatible // indirect
	github.com/aybabtme/rgbterm v0.0.0-20170906152045-cc83f3b3ce59 // indirect
	github.com/benbjohnson/clock v1.3.0 // indirect
	github.com/bramvdbogaerde/go-scp v1.2.0 // indirect
	github.com/cavaliergopher/grab v2.0.0+incompatible // indirect
	github.com/cloudflare/circl v1.6.0 // indirect
	github.com/codingsince1985/checksum v1.2.4 // indirect
	github.com/containerd/cgroups/v3 v3.0.5 // indirect
	github.com/containerd/console v1.0.4 // indirect
	github.com/containerd/continuity v0.4.5 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.16.3 // indirect
	github.com/containerd/typeurl/v2 v2.2.3 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.6 // indirect
	github.com/cyphar/filepath-securejoin v0.4.1 // indirect
	github.com/denisbrodbeck/machineid v1.0.1 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/djherbis/times v1.6.0 // indirect
	github.com/docker/cli v27.5.0+incompatible // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/docker/docker v27.5.1+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.8.2 // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/edsrzf/mmap-go v1.2.0 // indirect
	github.com/elliotwutingfeng/asciiset v0.0.0-20230602022725-51bbb787efab // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-bindata/go-bindata v3.1.2+incompatible // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.6.2 // indirect
	github.com/go-git/go-git/v5 v5.13.2 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/gofrs/flock v0.12.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/google/certificate-transparency-go v1.1.2 // indirect
	github.com/google/go-attestation v0.5.1 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/go-tpm v0.9.1 // indirect
	github.com/google/go-tspi v0.3.0 // indirect
	github.com/google/pprof v0.0.0-20250208200701-d0013a598941 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gookit/color v1.5.4 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.24.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/ipfs/go-log v1.0.5 // indirect
	github.com/ipfs/go-log/v2 v2.5.1 // indirect
	github.com/itchyny/gojq v0.12.17 // indirect
	github.com/itchyny/timefmt-go v0.1.6 // indirect
	github.com/jaypipes/ghw v0.13.0 // indirect
	github.com/jaypipes/pcidb v1.0.1 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/kendru/darwin/go/depgraph v0.0.0-20230809052043-4d1c7e9d1767 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/lithammer/fuzzysearch v1.1.8 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/mauromorales/xpasswd v0.4.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/mudler/entities v0.8.2 // indirect
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/otiai10/mint v1.6.3 // indirect
	github.com/packethost/packngo v0.29.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.3 // indirect
	github.com/phayes/freeport v0.0.0-20220201140144-74d24b5ae9f5 // indirect
	github.com/phayes/permbits v0.0.0-20190612203442-39d7c581d2ee // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pierrec/lz4/v4 v4.1.17 // indirect
	github.com/pjbgf/sha1cd v0.3.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/xattr v0.4.9 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/pterm/pterm v0.12.80 // indirect
	github.com/qeesung/image2ascii v1.0.1 // indirect
	github.com/rancher-sandbox/linuxkit v1.0.2 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/saferwall/pe v1.5.6 // indirect
	github.com/sagikazarmark/locafero v0.4.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/samber/lo v1.49.1 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1 // indirect
	github.com/satori/go.uuid v1.2.1-0.20181028125025-b2ce2384e17b // indirect
	github.com/secDre4mer/pkcs7 v0.0.0-20240322103146-665324a4461d // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/shirou/gopsutil/v4 v4.24.7 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/sirupsen/logrus v1.9.4-0.20241118143825-d1e633264448 // indirect
	github.com/skeema/knownhosts v1.3.1 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/swaggest/jsonschema-go v0.3.62 // indirect
	github.com/swaggest/refl v1.3.0 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/tredoe/osutil v1.5.0 // indirect
	github.com/twpayne/go-vfs/v4 v4.3.0 // indirect
	github.com/u-root/uio v0.0.0-20240209044354-b3d14b93376a // indirect
	github.com/ulikunitz/xz v0.5.11 // indirect
	github.com/vbatts/tar-split v0.11.6 // indirect
	github.com/vishvananda/netlink v1.3.0 // indirect
	github.com/vishvananda/netns v0.0.4 // indirect
	github.com/vmware/vmw-guestinfo v0.0.0-20220317130741-510905f0efa3 // indirect
	github.com/wayneashleyberry/terminal-dimensions v1.1.0 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/zcalusic/sysinfo v1.1.3 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.59.0 // indirect
	go.opentelemetry.io/otel v1.34.0 // indirect
	go.opentelemetry.io/otel/metric v1.34.0 // indirect
	go.opentelemetry.io/otel/trace v1.34.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.23.0 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/term v0.29.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	golang.org/x/tools v0.30.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250212204824-5a70512c5d8b // indirect
	google.golang.org/grpc v1.70.0 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	howett.net/plist v1.0.0 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/mount-utils v0.32.2 // indirect
	k8s.io/utils v0.0.0-20241104100929-3ea5e8cea738 // indirect
	pault.ag/go/modprobe v0.2.0 // indirect
	pault.ag/go/topsort v0.1.1 // indirect
)
