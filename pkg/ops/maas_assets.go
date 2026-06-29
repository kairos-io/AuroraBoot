package ops

import _ "embed"

// maasCurtinHook is the Kairos MAAS curtin-hook, baked into the OEM partition
// of a disk.maas image so MAAS/curtin runs it on deploy. Source:
// pkg/ops/assets/maas-curtin-hooks.
//
//go:embed assets/maas-curtin-hooks
var maasCurtinHook []byte
