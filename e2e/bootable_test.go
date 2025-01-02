package e2e_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"

	"github.com/gofrs/uuid"
	process "github.com/mudler/go-processmanager"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/spectrocloud/peg/matcher"
	"github.com/spectrocloud/peg/pkg/machine"
	"github.com/spectrocloud/peg/pkg/machine/types"
)

var _ = Describe("bootable artifacts", Label("bootable"), func() {
	var vm VM
	var err error

	BeforeEach(func() {
		_, ok := os.Stat(os.Getenv("ISO"))
		Expect(ok).To(BeNil(), "ISO should exist")
		vm, err = startVM()
		Expect(err).ToNot(HaveOccurred())
		vm.EventuallyConnects(1200)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			gatherLogs(vm)
			serial, _ := os.ReadFile(filepath.Join(vm.StateDir, "serial.log"))
			_ = os.MkdirAll("logs", os.ModePerm|os.ModeDir)
			_ = os.WriteFile(filepath.Join("logs", "serial.log"), serial, os.ModePerm)
			fmt.Println(string(serial))
		}

		err := vm.Destroy(nil)
		Expect(err).ToNot(HaveOccurred())
	})
	It("Should boot as expected", func() {
		By("Have secureboot enabled", func() {
			output, err := vm.Sudo("dmesg | grep -i secure")
			Expect(err).ToNot(HaveOccurred(), output)
			Expect(output).To(ContainSubstring("Secure boot enabled"))
		})

		By("Have our custom keys", func() {
			output, err := vm.Sudo("kairos-agent state get \"kairos.eficerts|tojson\"")
			Expect(err).ToNot(HaveOccurred(), output)
			// Check the test keys we created for this
			Expect(output).To(ContainSubstring("Kairos DB"))
			Expect(output).To(ContainSubstring("Kairos KEK"))
			Expect(output).To(ContainSubstring("Kairos PK"))
		})
	})
})

func emulateTPM(stateDir string) {
	t := path.Join(stateDir, "tpm")
	err := os.MkdirAll(t, os.ModePerm)
	Expect(err).ToNot(HaveOccurred())

	cmd := exec.Command("swtpm",
		"socket",
		"--tpmstate", fmt.Sprintf("dir=%s", t),
		"--ctrl", fmt.Sprintf("type=unixio,path=%s/swtpm-sock", t),
		"--tpm2", "--log", "level=20")
	err = cmd.Start()
	Expect(err).ToNot(HaveOccurred())

	err = os.WriteFile(path.Join(t, "pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0744)
	Expect(err).ToNot(HaveOccurred())
}

func startVM() (VM, error) {
	stateDir, err := os.MkdirTemp("", "")
	Expect(err).ToNot(HaveOccurred())
	fmt.Printf("State dir: %s\n", stateDir)

	opts := defaultVMOpts(stateDir)

	m, err := machine.New(opts...)
	Expect(err).ToNot(HaveOccurred())

	vm := NewVM(m, stateDir)
	_, err = vm.Start(context.Background())
	return vm, err
}

func defaultVMOpts(stateDir string) []types.MachineOption {
	opts := defaultVMOptsNoDrives(stateDir)

	driveSize := os.Getenv("DRIVE_SIZE")
	if driveSize == "" {
		driveSize = "25000"
	}

	opts = append(opts, types.WithDriveSize(driveSize))

	return opts
}

func defaultVMOptsNoDrives(stateDir string) []types.MachineOption {
	var err error

	if (os.Getenv("ISO") == "" && os.Getenv("RAW_IMAGE") == "") && os.Getenv("CREATE_VM") == "true" {
		fmt.Println("ISO or RAW_IMAGE missing")
		os.Exit(1)
	}

	var sshPort, spicePort int

	uid, _ := uuid.NewV4()
	vmName := uid.String()

	emulateTPM(stateDir)

	sshPort, err = getFreePort()
	Expect(err).ToNot(HaveOccurred())
	fmt.Printf("Using ssh port: %d\n", sshPort)

	memory := os.Getenv("MEMORY")
	if memory == "" {
		memory = "2096"
	}
	cpus := os.Getenv("CPUS")
	if cpus == "" {
		cpus = "2"
	}

	opts := []types.MachineOption{
		types.QEMUEngine,
		types.WithMemory(memory),
		types.WithCPU(cpus),
		types.WithSSHPort(strconv.Itoa(sshPort)),
		types.WithID(vmName),
		types.WithSSHUser("kairos"),
		types.WithSSHPass("kairos"),
		types.OnFailure(func(p *process.Process) {
			var serial string

			out, _ := os.ReadFile(p.StdoutPath())
			err, _ := os.ReadFile(p.StderrPath())
			status, _ := p.ExitCode()

			if serialBytes, err := os.ReadFile(path.Join(p.StateDir(), "serial.log")); err != nil {
				serial = fmt.Sprintf("Error reading serial log file: %s\n", err)
			} else {
				serial = string(serialBytes)
			}

			// We are explicitly killing the qemu process. We don't treat that as an error,
			// but we just print the output just in case.
			fmt.Printf("\nVM Aborted.\nstdout: %s\nstderr: %s\nserial: %s\nExit status: %s\n", out, err, serial, status)
			Fail(fmt.Sprintf("\nVM Aborted.\nstdout: %s\nstderr: %s\nserial: %s\nExit status: %s\n",
				out, err, serial, status))
		}),
		types.WithStateDir(stateDir),
		// Serial output to file: https://superuser.com/a/1412150
		func(m *types.MachineConfig) error {
			m.Args = append(m.Args,
				"-chardev", fmt.Sprintf("stdio,mux=on,id=char0,logfile=%s,signal=off", path.Join(stateDir, "serial.log")),
				"-serial", "chardev:char0",
				"-mon", "chardev=char0",
			)
			m.Args = append(m.Args,
				"-chardev", fmt.Sprintf("socket,id=chrtpm,path=%s/swtpm-sock", path.Join(stateDir, "tpm")),
				"-tpmdev", "emulator,id=tpm0,chardev=chrtpm", "-device", "tpm-tis,tpmdev=tpm0",
			)
			return nil
		},
		// Firmware
		func(m *types.MachineConfig) error {
			FW := os.Getenv("FIRMWARE")
			if FW != "" {
				getwd, err := os.Getwd()
				if err != nil {
					return err
				}
				m.Args = append(m.Args, "-drive",
					fmt.Sprintf("file=%s,if=pflash,format=raw,readonly=on", FW),
				)
				// Efivars empty is to boot in setup mode, good for testing UKI and auto enrollment
				if os.Getenv("EFIVARS_EMPTY") == "true" {
					// Copy the empty efivars to not modify it
					f, err := os.ReadFile(filepath.Join(getwd, "assets/efivars.empty.fd"))
					if err != nil {
						return err
					}
					err = os.WriteFile(filepath.Join(stateDir, "efivars.empty.fd"), f, os.ModePerm)
					if err != nil {
						return err
					}

					m.Args = append(m.Args, "-drive",
						fmt.Sprintf("file=%s,if=pflash,format=raw", filepath.Join(stateDir, "efivars.empty.fd")),
					)
				} else {
					// This uses the efivars.fd file that has the default keys from Microsoft, useful to test secureboot out of the box
					f, err := os.ReadFile(filepath.Join(getwd, "assets/efivars.fd"))
					if err != nil {
						return err
					}
					err = os.WriteFile(filepath.Join(stateDir, "efivars.fd"), f, os.ModePerm)
					if err != nil {
						return err
					}

					m.Args = append(m.Args, "-drive",
						fmt.Sprintf("file=%s,if=pflash,format=raw", filepath.Join(stateDir, "efivars.fd")),
					)
				}

				// Needed to be set for secureboot!
				m.Args = append(m.Args, "-machine", "q35,smm=on")
			}

			return nil
		},
		types.WithDataSource(os.Getenv("DATASOURCE")),
	}
	if os.Getenv("ISO") != "" {
		opts = append(opts, types.WithISO(os.Getenv("ISO")))
	}
	if os.Getenv("RAW_IMAGE") != "" {
		opts = append(opts, types.WithDrive(os.Getenv("RAW_IMAGE")))
	}
	if os.Getenv("KVM") != "" {
		opts = append(opts, func(m *types.MachineConfig) error {
			m.Args = append(m.Args,
				"-enable-kvm",
			)
			return nil
		})
	}

	if os.Getenv("USE_QEMU") == "true" {
		opts = append(opts, types.QEMUEngine)

		// You can connect to it with "spicy" or other tool.
		// DISPLAY is already taken on Linux X sessions
		if os.Getenv("MACHINE_SPICY") != "" {
			spicePort, _ = getFreePort()
			for spicePort == sshPort { // avoid collision
				spicePort, _ = getFreePort()
			}
			display := fmt.Sprintf("-spice port=%d,addr=127.0.0.1,disable-ticketing=yes", spicePort)
			opts = append(opts, types.WithDisplay(display))

			cmd := exec.Command("spicy",
				"-h", "127.0.0.1",
				"-p", strconv.Itoa(spicePort))
			err = cmd.Start()
			Expect(err).ToNot(HaveOccurred())
		}
	} else {
		opts = append(opts, types.VBoxEngine)
	}

	return opts
}

func getFreePort() (port int, err error) {
	var a *net.TCPAddr
	if a, err = net.ResolveTCPAddr("tcp", "localhost:0"); err == nil {
		var l *net.TCPListener
		if l, err = net.ListenTCP("tcp", a); err == nil {
			defer l.Close()
			return l.Addr().(*net.TCPAddr).Port, nil
		}
	}
	return
}

func gatherLogs(vm VM) {
	vm.Scp("assets/kubernetes_logs.sh", "/tmp/logs.sh", "0770")
	vm.Sudo("sh /tmp/logs.sh > /run/kube_logs")
	vm.Sudo("cat /oem/* > /run/oem.yaml")
	vm.Sudo("cat /etc/resolv.conf > /run/resolv.conf")
	vm.Sudo("k3s kubectl get pods -A -o json > /run/pods.json")
	vm.Sudo("k3s kubectl get events -A -o json > /run/events.json")
	vm.Sudo("cat /proc/cmdline > /run/cmdline")
	vm.Sudo("chmod 777 /run/events.json")

	vm.Sudo("df -h > /run/disk")
	vm.Sudo("mount > /run/mounts")
	vm.Sudo("blkid > /run/blkid")
	vm.Sudo("dmesg > /run/dmesg.log")

	// zip all files under /var/log/kairos
	vm.Sudo("tar -czf /run/kairos-agent-logs.tar.gz /var/log/kairos")

	vm.GatherAllLogs(
		[]string{
			"edgevpn@kairos",
			"kairos-agent",
			"cos-setup-boot",
			"cos-setup-network",
			"cos-setup-reconcile",
			"kairos",
			"k3s",
			"k3s-agent",
		},
		[]string{
			"/var/log/edgevpn.log",
			"/var/log/kairos/agent.log",
			"/run/pods.json",
			"/run/disk",
			"/run/mounts",
			"/run/blkid",
			"/run/events.json",
			"/run/kube_logs",
			"/run/cmdline",
			"/run/oem.yaml",
			"/run/resolv.conf",
			"/run/dmesg.log",
			"/run/immucore/immucore.log",
			"/run/immucore/initramfs_stage.log",
			"/run/immucore/rootfs_stage.log",
			"/tmp/ovmf_debug.log",
			"/run/kairos-agent-logs.tar.gz",
		})
}
