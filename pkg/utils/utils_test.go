/*
Copyright © 2021 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils_test

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kairos-io/enki/pkg/constants"
	"github.com/kairos-io/enki/pkg/utils"
	v1mock "github.com/kairos-io/kairos-agent/v2/tests/mocks"
	sdkTypes "github.com/kairos-io/kairos-sdk/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/twpayne/go-vfs/v5"
	"github.com/twpayne/go-vfs/v5/vfst"
)

var _ = Describe("Utils", Label("utils"), func() {
	var runner *v1mock.FakeRunner
	var logger sdkTypes.KairosLogger
	var fs vfs.FS
	var cleanup func()

	BeforeEach(func() {
		runner = v1mock.NewFakeRunner()
		logger = sdkTypes.NewNullLogger()
		// Ensure /tmp exists in the VFS
		fs, cleanup, _ = vfst.NewTestFS(nil)
		fs.Mkdir("/tmp", constants.DirPerm)
		fs.Mkdir("/run", constants.DirPerm)
		fs.Mkdir("/etc", constants.DirPerm)

	})
	AfterEach(func() { cleanup() })
	Describe("CopyFile", Label("CopyFile"), func() {
		It("Copies source file to target file", func() {
			err := utils.MkdirAll(fs, "/some", constants.DirPerm)
			Expect(err).ShouldNot(HaveOccurred())
			_, err = fs.Create("/some/file")
			Expect(err).ShouldNot(HaveOccurred())
			_, err = fs.Stat("/some/otherfile")
			Expect(err).Should(HaveOccurred())
			Expect(utils.CopyFile(fs, "/some/file", "/some/otherfile")).ShouldNot(HaveOccurred())
			e, err := utils.Exists(fs, "/some/otherfile")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(e).To(BeTrue())
		})
		It("Copies source file to target folder", func() {
			err := utils.MkdirAll(fs, "/some", constants.DirPerm)
			Expect(err).ShouldNot(HaveOccurred())
			err = utils.MkdirAll(fs, "/someotherfolder", constants.DirPerm)
			Expect(err).ShouldNot(HaveOccurred())
			_, err = fs.Create("/some/file")
			Expect(err).ShouldNot(HaveOccurred())
			_, err = fs.Stat("/someotherfolder/file")
			Expect(err).Should(HaveOccurred())
			Expect(utils.CopyFile(fs, "/some/file", "/someotherfolder")).ShouldNot(HaveOccurred())
			e, err := utils.Exists(fs, "/someotherfolder/file")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(e).To(BeTrue())
		})
		It("Fails to open non existing file", func() {
			err := utils.MkdirAll(fs, "/some", constants.DirPerm)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(utils.CopyFile(fs, "/some/file", "/some/otherfile")).NotTo(BeNil())
			_, err = fs.Stat("/some/otherfile")
			Expect(err).NotTo(BeNil())
		})
		It("Fails to copy on non writable target", func() {
			err := utils.MkdirAll(fs, "/some", constants.DirPerm)
			Expect(err).ShouldNot(HaveOccurred())
			fs.Create("/some/file")
			_, err = fs.Stat("/some/otherfile")
			Expect(err).NotTo(BeNil())
			fs = vfs.NewReadOnlyFS(fs)
			Expect(utils.CopyFile(fs, "/some/file", "/some/otherfile")).NotTo(BeNil())
			_, err = fs.Stat("/some/otherfile")
			Expect(err).NotTo(BeNil())
		})
	})
	Describe("CreateDirStructure", Label("CreateDirStructure"), func() {
		It("Creates essential directories", func() {
			dirList := []string{"sys", "proc", "dev", "tmp", "boot", "usr/local", "oem"}
			for _, dir := range dirList {
				_, err := fs.Stat(fmt.Sprintf("/my/root/%s", dir))
				Expect(err).NotTo(BeNil())
			}
			Expect(utils.CreateDirStructure(fs, "/my/root")).To(BeNil())
			for _, dir := range dirList {
				fi, err := fs.Stat(fmt.Sprintf("/my/root/%s", dir))
				Expect(err).To(BeNil())
				if fi.Name() == "tmp" {
					Expect(fmt.Sprintf("%04o", fi.Mode().Perm())).To(Equal("0777"))
					Expect(fi.Mode() & os.ModeSticky).NotTo(Equal(0))
				}
				if fi.Name() == "sys" {
					Expect(fmt.Sprintf("%04o", fi.Mode().Perm())).To(Equal("0555"))
				}
			}
		})
		It("Fails on non writable target", func() {
			fs = vfs.NewReadOnlyFS(fs)
			Expect(utils.CreateDirStructure(fs, "/my/root")).NotTo(BeNil())
		})
	})
	Describe("DirSize", Label("fs"), func() {
		BeforeEach(func() {
			err := utils.MkdirAll(fs, "/folder/subfolder", constants.DirPerm)
			Expect(err).ShouldNot(HaveOccurred())
			f, err := fs.Create("/folder/file")
			Expect(err).ShouldNot(HaveOccurred())
			err = f.Truncate(1024)
			Expect(err).ShouldNot(HaveOccurred())
			f, err = fs.Create("/folder/subfolder/file")
			Expect(err).ShouldNot(HaveOccurred())
			err = f.Truncate(2048)
			Expect(err).ShouldNot(HaveOccurred())
		})
		It("Returns the expected size of a test folder", func() {
			size, err := utils.DirSize(fs, "/folder")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(size).To(Equal(int64(3072)))
		})
	})
	Describe("CalcFileChecksum", Label("checksum"), func() {
		It("compute correct sha256 checksum", func() {
			testData := strings.Repeat("abcdefghilmnopqrstuvz\n", 20)
			testDataSHA256 := "7f182529f6362ae9cfa952ab87342a7180db45d2c57b52b50a68b6130b15a422"

			err := fs.Mkdir("/iso", constants.DirPerm)
			Expect(err).ShouldNot(HaveOccurred())
			err = fs.WriteFile("/iso/test.iso", []byte(testData), 0644)
			Expect(err).ShouldNot(HaveOccurred())

			checksum, err := utils.CalcFileChecksum(fs, "/iso/test.iso")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(checksum).To(Equal(testDataSHA256))
		})
	})
	Describe("CreateSquashFS", Label("CreateSquashFS"), func() {
		It("runs with no options if none given", func() {
			err := utils.CreateSquashFS(runner, logger, "source", "dest", []string{})
			Expect(runner.IncludesCmds([][]string{
				{"mksquashfs", "source", "dest"},
			})).To(BeNil())
			Expect(err).ToNot(HaveOccurred())
		})
		It("runs with options if given", func() {
			err := utils.CreateSquashFS(runner, logger, "source", "dest", constants.GetDefaultSquashfsOptions())
			cmd := []string{"mksquashfs", "source", "dest"}
			cmd = append(cmd, constants.GetDefaultSquashfsOptions()...)
			Expect(runner.IncludesCmds([][]string{
				cmd,
			})).To(BeNil())
			Expect(err).ToNot(HaveOccurred())
		})
		It("returns an error if it fails", func() {
			runner.ReturnError = errors.New("error")
			err := utils.CreateSquashFS(runner, logger, "source", "dest", []string{})
			Expect(runner.IncludesCmds([][]string{
				{"mksquashfs", "source", "dest"},
			})).To(BeNil())
			Expect(err).To(HaveOccurred())
		})
	})
	Describe("GetUkiCmdline", Label("GetUkiCmdline"), func() {
		var defaultCmdline string
		BeforeEach(func() {
			defaultCmdline = constants.UkiCmdline + " " + constants.UkiCmdlineInstall
		})

		It("returns the default cmdline", func() {
			entries := utils.GetUkiCmdline()
			Expect(entries[0].Cmdline).To(Equal(defaultCmdline))
		})

		It("returns the default cmdline with the cmdline flag and install-mode", func() {
			viper.Set("extra-cmdline", []string{"key=value testkey"})
			entries := utils.GetUkiCmdline()
			cmdlines := []string{}
			for _, entry := range entries {
				cmdlines = append(cmdlines, entry.Cmdline)
			}
			Expect(cmdlines).To(ContainElements(defaultCmdline))
			Expect(cmdlines).To(ContainElements(defaultCmdline + " key=value testkey"))
		})

		It("returns more than one cmdline with the cmdline flag if specified multiple values", func() {
			viper.Set("extra-cmdline", []string{"key=value testkey", "another=value anotherkey"})
			entries := utils.GetUkiCmdline()
			cmdlines := []string{}
			for _, entry := range entries {
				cmdlines = append(cmdlines, entry.Cmdline)
			}

			// Should contain the default one
			Expect(cmdlines).To(ContainElements(defaultCmdline))
			// Also the extra ones, without the install-mode
			Expect(cmdlines).To(ContainElements(defaultCmdline + " key=value testkey"))
			Expect(cmdlines).To(ContainElements(defaultCmdline + " another=value anotherkey"))
		})

		It("expands the default cmdline if extended-cmdline is used", func() {
			viper.Set("extend-cmdline", "key=value testkey")
			entries := utils.GetUkiCmdline()
			for _, entry := range entries {
				Expect(entry.Cmdline).To(MatchRegexp(".*key=value testkey"))
			}
		})
	})

	Describe("GetUkiSingleCmdlines", Label("GetUkiSingleCmdlines"), func() {
		var defaultCmdline string
		BeforeEach(func() {
			defaultCmdline = constants.UkiCmdline + " " + constants.UkiCmdlineInstall
		})

		It("returns the specified entry", func() {
			viper.Set("single-efi-cmdline", []string{"My Entry: key=value"})
			viper.Set("boot-branding", "Kairos")

			entries := utils.GetUkiSingleCmdlines(sdkTypes.NewNullLogger())
			Expect(entries[0].Cmdline).To(MatchRegexp(defaultCmdline + "  key=value"))
			Expect(entries[0].Title).To(ContainSubstring("Kairos (My Entry)"))
			Expect(entries[0].FileName).To(Equal("My_Entry"))
		})
	})
})
