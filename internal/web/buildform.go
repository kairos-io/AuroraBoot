package web

import (
	"fmt"

	"github.com/chasefleming/elem-go"
	"github.com/chasefleming/elem-go/attrs"
)

// BuildForm represents a form builder for Kairos factory configuration
type BuildForm struct {
	BaseImage    string
	Architecture string
	Model        string
	Variant      string
	Version      string
	CloudConfig  string
	ArtifactRaw  string
	ArtifactISO  string
	ArtifactTAR  string
}

// New creates a new BuildForm instance
func NewBuildForm(formData map[string]string) *BuildForm {
	bf := &BuildForm{
		BaseImage:    formData["base_image"],
		Architecture: formData["architecture"],
		Model:        formData["model"],
		Variant:      formData["variant"],
		Version:      formData["version"],
		CloudConfig:  formData["cloud_config"],
		ArtifactRaw:  formData["artifact_raw"],
		ArtifactISO:  formData["artifact_iso"],
		ArtifactTAR:  formData["artifact_tar"],
	}

	// Set defaults if values are empty
	if bf.BaseImage == "" {
		bf.BaseImage = "ubuntu:24.04"
	}
	if bf.Architecture == "" {
		bf.Architecture = "amd64"
	}
	if bf.Model == "" {
		bf.Model = "generic"
	}
	if bf.Variant == "" {
		bf.Variant = "core"
	}
	if bf.Version == "" {
		bf.Version = "v0.1.0-alpha"
	}
	// Set default artifact values - RAW is always checked by default
	if bf.ArtifactRaw == "" {
		bf.ArtifactRaw = "on"
	}

	return bf
}

// AdaptTo changes the form to be valid for the given field based on its current value
func (bf *BuildForm) AdaptTo(field string) {
	switch field {
	case "architecture":
		// If architecture is AMD64, force model to generic
		if bf.Architecture == "amd64" {
			bf.Model = "generic"
		}
		// If architecture is ARM64, force model to rpi4 if current is generic
		if bf.Architecture == "arm64" && bf.Model == "generic" {
			bf.Model = "rpi4"
		}
	case "model":
		// If model is generic, force architecture to AMD64
		if bf.Model == "generic" {
			bf.Architecture = "amd64"
		}
		// If model is ARM64 model, force architecture to ARM64
		if bf.Model != "generic" {
			bf.Architecture = "arm64"
		}
	}
}

// Render generates the HTML form string
func (bf *BuildForm) Render() string {
	form := bf.buildForm()
	return form.Render()
}

// buildForm creates the form element with all its fields
func (bf *BuildForm) buildForm() *elem.Element {
	return elem.Form(attrs.Props{
		attrs.ID:    "build-form",
		attrs.Class: "mt-8 space-y-6",
	},
		bf.baseImageSection(),
		bf.architectureSection(),
		bf.modelSection(),
		bf.variantSection(),
		bf.versionSection(),
		bf.customOptionsSection(),
		bf.artifactsSection(),
	)
}

// baseImageSection creates the base image selection section
func (bf *BuildForm) baseImageSection() *elem.Element {
	return elem.Div(attrs.Props{
		attrs.Class: "accordion-section",
	},
		// Accordion header
		elem.H2(attrs.Props{
			attrs.ID: "accordion-heading-base-image",
		},
			elem.Button(attrs.Props{
				attrs.Type:              "button",
				attrs.Class:             "flex items-center justify-between w-full p-5 font-medium text-gray-500 border border-b border-gray-200 mb-0 rounded-t-xl focus:ring-4 focus:ring-gray-200 dark:focus:ring-gray-800 dark:border-gray-700 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 gap-3 aria-expanded:true:border-b-0",
				"data-accordion-target": "#accordion-body-base-image",
				"aria-expanded":         "true",
				"aria-controls":         "accordion-body-base-image",
			},
				elem.Span(attrs.Props{}, elem.Text("Base Image")),
				bf.createAccordionIcon(),
			),
		),
		// Accordion body
		elem.Div(attrs.Props{
			attrs.ID:          "accordion-body-base-image",
			attrs.Class:       "p-5 border border-b-0 border-gray-200 dark:border-gray-700 dark:bg-gray-900 mt-0",
			"aria-labelledby": "accordion-heading-base-image",
		},
			elem.Ul(attrs.Props{
				attrs.Class: "grid w-full gap-3 md:grid-cols-3 baseimage-list",
			},
				bf.createBaseImageOption("ubuntu", "Ubuntu 24.04 LTS", "ubuntu:24.04", "assets/img/ubuntu.svg", true),
				bf.createBaseImageOption("fedora", "Fedora 40", "fedora:40", "assets/img/fedora.svg", false),
				bf.createBaseImageOption("opensuse", "openSUSE Leap 15.6", "opensuse/leap:15.6", "assets/img/opensuse.svg", false),
				bf.createBaseImageOption("debian", "Debian 12 (Bookworm)", "debian:12", "assets/img/debian.svg", false),
				bf.createBaseImageOption("alpine", "Alpine 3.21", "alpine:3.21", "assets/img/alpine.svg", false),
				bf.createBaseImageOption("rockylinux", "Rocky Linux 9", "rockylinux:9", "assets/img/rockylinux.svg", false),
			),
		),
	)
}

// architectureSection creates the architecture selection section
func (bf *BuildForm) architectureSection() *elem.Element {
	return elem.Div(attrs.Props{
		attrs.Class: "accordion-section",
	},
		// Accordion header
		elem.H2(attrs.Props{
			attrs.ID: "accordion-heading-architecture",
		},
			elem.Button(attrs.Props{
				attrs.Type:              "button",
				attrs.Class:             "flex items-center justify-between w-full p-5 font-medium text-gray-500 border border-b border-gray-200 mb-0 focus:ring-4 focus:ring-gray-200 dark:focus:ring-gray-800 dark:border-gray-700 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 gap-3 aria-expanded:true:border-b-0",
				"data-accordion-target": "#accordion-body-architecture",
				"aria-expanded":         "true",
				"aria-controls":         "accordion-body-architecture",
			},
				elem.Span(attrs.Props{}, elem.Text("Architecture")),
				bf.createAccordionIcon(),
			),
		),
		// Accordion body
		elem.Div(attrs.Props{
			attrs.ID:          "accordion-body-architecture",
			attrs.Class:       "p-5 border border-b-0 border-gray-200 dark:border-gray-700 mt-0",
			"aria-labelledby": "accordion-heading-architecture",
		},
			elem.Ul(attrs.Props{
				attrs.Class: "grid w-full gap-6 md:grid-cols-2 arch-list",
			},
				bf.createArchitectureOption("amd64", "AMD64", "assets/img/amd.svg", "For devices using the x86-64 (AMD64) architecture, commonly found in Intel and AMD processors.", true),
				bf.createArchitectureOption("arm64", "ARM64", "assets/img/arm.svg", "For devices using the 64-bit ARM (AArch64) architecture.", false),
			),
		),
	)
}

// versionSection creates the version input section
func (bf *BuildForm) versionSection() *elem.Element {
	return elem.Div(attrs.Props{
		attrs.Class: "accordion-section",
	},
		// Accordion header
		elem.H2(attrs.Props{
			attrs.ID: "accordion-heading-version",
		},
			elem.Button(attrs.Props{
				attrs.Type:              "button",
				attrs.Class:             "flex items-center justify-between w-full p-5 font-medium text-gray-500 border border-b border-gray-200 mb-0 focus:ring-4 focus:ring-gray-200 dark:focus:ring-gray-800 dark:border-gray-700 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 gap-3 aria-expanded:true:border-b-0",
				"data-accordion-target": "#accordion-body-version",
				"aria-expanded":         "true",
				"aria-controls":         "accordion-body-version",
			},
				elem.Span(attrs.Props{}, elem.Text("Version")),
				bf.createAccordionIcon(),
			),
		),
		// Accordion body
		elem.Div(attrs.Props{
			attrs.ID:          "accordion-body-version",
			attrs.Class:       "p-5 border border-gray-200 dark:border-gray-700 mt-0",
			"aria-labelledby": "accordion-heading-version",
		},
			elem.Input(bf.mergeAttrs(attrs.Props{
				attrs.Type:     "text",
				attrs.Name:     "version",
				attrs.ID:       "version",
				attrs.Value:    bf.getSelectedValue("version"),
				attrs.Class:    "bg-gray-50 border border-gray-300 text-gray-900 text-sm rounded-lg focus:ring-blue-500 focus:border-blue-500 block w-full p-2.5 dark:bg-gray-700 dark:border-gray-600 dark:placeholder-gray-400 dark:text-white dark:focus:ring-blue-500 dark:focus:border-blue-500",
				"placeholder":  "v0.1.0-alpha",
				attrs.Required: "true",
			}, bf.getHtmxAttrs("version", "keyup changed delay:100ms")),
			),
		),
	)
}

// customOptionsSection creates the custom options textarea section
func (bf *BuildForm) customOptionsSection() *elem.Element {
	return elem.Div(attrs.Props{
		attrs.Class: "accordion-section",
	},
		// Accordion header
		elem.H2(attrs.Props{
			attrs.ID: "accordion-heading-configuration",
		},
			elem.Button(attrs.Props{
				attrs.Type:              "button",
				attrs.Class:             "flex items-center justify-between w-full p-5 font-medium text-gray-500 border border-b border-gray-200 focus:ring-4 focus:ring-gray-200 dark:focus:ring-gray-800 dark:border-gray-700 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 gap-3 aria-expanded:true:border-b-0",
				"data-accordion-target": "#accordion-body-configuration",
				"aria-expanded":         "true",
				"aria-controls":         "accordion-body-configuration",
			},
				elem.Span(attrs.Props{}, elem.Text("Configuration")),
				elem.Span(attrs.Props{
					attrs.Class: "selected-value flex items-center gap-2 ml-auto min-w-0 text-gray-900 dark:text-white",
				},
					elem.Span(attrs.Props{
						attrs.Class: "truncate",
					}, elem.Text("none")),
				),
				bf.createAccordionIcon(),
			),
		),
		// Accordion body
		elem.Div(attrs.Props{
			attrs.ID:          "accordion-body-configuration",
			attrs.Class:       "p-5 border border-b-0 border-gray-200 dark:border-gray-700",
			"aria-labelledby": "accordion-heading-configuration",
		},
			elem.Label(attrs.Props{
				attrs.For:   "cloud_config",
				attrs.Class: "block mb-2 text-sm font-medium text-gray-900 dark:text-white",
			}, elem.Text("Paste your "), elem.Code(attrs.Props{}, elem.Text("cloud-config.yaml")), elem.Text(" here (optional):")),
			elem.Textarea(bf.mergeAttrs(attrs.Props{
				attrs.ID:       "cloud_config",
				attrs.Name:     "cloud_config",
				attrs.Rows:     "10",
				attrs.Class:    "font-mono w-full p-4 rounded-lg bg-gray-900 text-green-400 border border-gray-700 focus:ring-blue-500 focus:border-blue-500 resize-vertical shadow-inner",
				"placeholder":  "#cloud-config",
				"spellcheck":   "false",
				"autocomplete": "off",
			}, bf.getHtmxAttrs("cloud_config", "keyup changed delay:100ms")),
				elem.Text(bf.getSelectedValue("cloud_config")),
			),
		),
	)
}

// modelSection creates the model selection section
func (bf *BuildForm) modelSection() *elem.Element {
	return elem.Div(attrs.Props{
		attrs.Class: "accordion-section",
	},
		// Accordion header
		elem.H2(attrs.Props{
			attrs.ID: "accordion-heading-model",
		},
			elem.Button(attrs.Props{
				attrs.Type:              "button",
				attrs.Class:             "flex items-center justify-between w-full p-5 font-medium text-gray-500 border border-b border-gray-200 mb-0 focus:ring-4 focus:ring-gray-200 dark:focus:ring-gray-800 dark:border-gray-700 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 gap-3 aria-expanded:true:border-b-0",
				"data-accordion-target": "#accordion-body-model",
				"aria-expanded":         "true",
				"aria-controls":         "accordion-body-model",
			},
				elem.Span(attrs.Props{}, elem.Text("Model")),
				bf.createAccordionIcon(),
			),
		),
		// Accordion body
		elem.Div(attrs.Props{
			attrs.ID:          "accordion-body-model",
			attrs.Class:       "p-5 border border-b-0 border-gray-200 dark:border-gray-700 mt-0",
			"aria-labelledby": "accordion-heading-model",
		},
			elem.Ul(attrs.Props{
				attrs.Class: "grid w-full gap-6 md:grid-cols-2 model-list",
			},
				bf.createModelOption("generic", "Generic", "assets/img/amd.svg", "For generic x86-64 systems", true),
				bf.createModelOption("rpi3", "Raspberry Pi 3", "assets/img/arm.svg", "For Raspberry Pi 3 Model B/B+", false),
				bf.createModelOption("rpi4", "Raspberry Pi 4", "assets/img/arm.svg", "For Raspberry Pi 4 Model B", false),
				bf.createModelOption("rock64", "ROCK64", "assets/img/arm.svg", "For ROCK64 single-board computer", false),
				bf.createModelOption("pine64", "Pine64", "assets/img/arm.svg", "For Pine64 single-board computer", false),
				bf.createModelOption("odroid-n2", "ODROID-N2", "assets/img/arm.svg", "For ODROID-N2 single-board computer", false),
			),
		),
	)
}

// variantSection creates the variant selection section
func (bf *BuildForm) variantSection() *elem.Element {
	return elem.Div(attrs.Props{
		attrs.Class: "accordion-section",
	},
		// Accordion header
		elem.H2(attrs.Props{
			attrs.ID: "accordion-heading-variant",
		},
			elem.Button(attrs.Props{
				attrs.Type:              "button",
				attrs.Class:             "flex items-center justify-between w-full p-5 font-medium text-gray-500 border border-b border-gray-200 mb-0 focus:ring-4 focus:ring-gray-200 dark:focus:ring-gray-800 dark:border-gray-700 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 gap-3 aria-expanded:true:border-b-0",
				"data-accordion-target": "#accordion-body-variant",
				"aria-expanded":         "true",
				"aria-controls":         "accordion-body-variant",
			},
				elem.Span(attrs.Props{}, elem.Text("Variant")),
				bf.createAccordionIcon(),
			),
		),
		// Accordion body
		elem.Div(attrs.Props{
			attrs.ID:          "accordion-body-variant",
			attrs.Class:       "p-5 border border-b-0 border-gray-200 dark:border-gray-700 mt-0",
			"aria-labelledby": "accordion-heading-variant",
		},
			elem.Ul(attrs.Props{
				attrs.Class: "grid w-full gap-6 md:grid-cols-2 variant-list",
			},
				bf.createVariantOption("core", "Core", "assets/img/amd.svg", "Minimal system with essential components only", true),
				bf.createVariantOption("standard", "Standard", "assets/img/amd.svg", "Full system with all components and tools", false),
			),
		),
	)
}

// artifactsSection creates the artifacts selection section
func (bf *BuildForm) artifactsSection() *elem.Element {
	return elem.Div(attrs.Props{
		attrs.Class: "accordion-section",
	},
		// Accordion header
		elem.H2(attrs.Props{
			attrs.ID: "accordion-heading-artifacts",
		},
			elem.Button(attrs.Props{
				attrs.Type:              "button",
				attrs.Class:             "flex items-center justify-between w-full p-5 font-medium text-gray-500 border border-b border-gray-200 mb-0 focus:ring-4 focus:ring-gray-200 dark:focus:ring-gray-800 dark:border-gray-700 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 gap-3 aria-expanded:true:border-b-0",
				"data-accordion-target": "#accordion-body-artifacts",
				"aria-expanded":         "true",
				"aria-controls":         "accordion-body-artifacts",
			},
				elem.Span(attrs.Props{}, elem.Text("Artifacts")),
				bf.createAccordionIcon(),
			),
		),
		// Accordion body
		elem.Div(attrs.Props{
			attrs.ID:          "accordion-body-artifacts",
			attrs.Class:       "p-5 border border-gray-200 dark:border-gray-700",
			"aria-labelledby": "accordion-heading-artifacts",
		},
			elem.Ul(attrs.Props{
				attrs.Class: "grid w-full gap-6 md:grid-cols-3",
			},
				bf.createArtifactOption("artifact_raw", "RAW", "assets/img/raw.svg", "assets/img/aws.svg", "(Always generated) Ready to copy to a USB, SD card, board or use with AWS", true, true),
				bf.createArtifactOption("artifact_iso", "ISO", "assets/img/iso.svg", "", "Bootable ISO image for CD/DVD or USB", false, false),
				bf.createArtifactOption("artifact_tar", "TAR", "assets/img/tar.svg", "", "Compressed archive for deployment", false, false),
			),
		),
	)
}

// Helper methods

// createBaseImageOption creates a base image option with the same styling as the original
func (bf *BuildForm) createBaseImageOption(id, label, value, imageSrc string, isSelected bool) *elem.Element {
	selected := bf.isSelected("base_image", value) || (isSelected && bf.getSelectedValue("base_image") == "")

	checkedAttr := ""
	if selected {
		checkedAttr = "checked"
	}

	return elem.Li(attrs.Props{},
		elem.Input(bf.mergeAttrs(attrs.Props{
			attrs.Type:     "radio",
			attrs.Name:     "base_image",
			attrs.ID:       id + "-option",
			attrs.Value:    value,
			attrs.Class:    "hidden peer",
			attrs.Required: "true",
			"checked":      checkedAttr,
		}, bf.getHtmxAttrs("base_image", "change")),
		),
		elem.Label(attrs.Props{
			attrs.For:   id + "-option",
			attrs.Class: "inline-flex items-center justify-between w-full p-2 text-gray-500 bg-white border-2 border-gray-200 rounded-lg cursor-pointer dark:hover:text-gray-300 dark:border-gray-700 peer-checked:border-blue-600 dark:peer-checked:border-blue-600 hover:text-gray-600 dark:peer-checked:text-gray-300 peer-checked:text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:bg-gray-800 dark:hover:bg-gray-700",
		},
			elem.Div(attrs.Props{
				attrs.Class: "flex items-center space-x-3",
				"data-js":   "option-container",
			},
				elem.Img(attrs.Props{
					attrs.Src:   imageSrc,
					attrs.Class: "mb-2 w-7 h-7 text-sky-500 relative top-[5px]",
				}),
				elem.Div(attrs.Props{
					attrs.Class: "text-m font-semibold leading-none",
					"data-js":   "option-label",
				}, elem.Text(label)),
			),
		),
	)
}

// createArchitectureOption creates an architecture option with the same styling as the original
func (bf *BuildForm) createArchitectureOption(id, label, imageSrc, description string, isSelected bool) *elem.Element {
	selected := bf.isSelected("architecture", id) || (isSelected && bf.getSelectedValue("architecture") == "")

	checkedAttr := ""
	if selected {
		checkedAttr = "checked"
	}

	return elem.Li(attrs.Props{},
		elem.Input(bf.mergeAttrs(attrs.Props{
			attrs.Type:     "radio",
			attrs.Name:     "architecture",
			attrs.ID:       id + "-option",
			attrs.Value:    id,
			attrs.Class:    "hidden peer",
			attrs.Required: "true",
			"checked":      checkedAttr,
		}, bf.getHtmxAttrs("architecture", "change")),
		),
		elem.Label(attrs.Props{
			attrs.For:   id + "-option",
			attrs.Class: "inline-flex items-center justify-between w-full p-3 text-gray-500 bg-white border-2 border-gray-200 rounded-lg cursor-pointer dark:hover:text-gray-300 dark:border-gray-700 peer-checked:border-blue-600 dark:peer-checked:border-blue-600 hover:text-gray-600 dark:peer-checked:text-gray-300 peer-checked:text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:bg-gray-800 dark:hover:bg-gray-700",
		},
			elem.Div(attrs.Props{
				attrs.Class: "block",
			},
				elem.Img(attrs.Props{
					attrs.Src:   imageSrc,
					attrs.Alt:   label + " Logo",
					"data-js":   "option-label",
					attrs.Class: "w-14 h-14",
				}),
				elem.Div(attrs.Props{
					attrs.Class: "w-full text-sm",
				}, elem.Text(description)),
			),
		),
	)
}

// createModelOption creates a model option with the same styling as the original
func (bf *BuildForm) createModelOption(id, label, imageSrc, description string, isSelected bool) *elem.Element {
	selected := bf.isSelected("model", id) || (isSelected && bf.getSelectedValue("model") == "")

	checkedAttr := ""
	if selected {
		checkedAttr = "checked"
	}

	return elem.Li(attrs.Props{
		attrs.Class: "model-option arm-only hidden",
	},
		elem.Input(bf.mergeAttrs(attrs.Props{
			attrs.Type:     "radio",
			attrs.Name:     "model",
			attrs.ID:       id + "-option",
			attrs.Value:    id,
			attrs.Class:    "hidden peer",
			attrs.Required: "true",
			"checked":      checkedAttr,
		}, bf.getHtmxAttrs("model", "change")),
		),
		elem.Label(attrs.Props{
			attrs.For:   id + "-option",
			attrs.Class: "inline-flex items-center justify-between w-full p-3 text-gray-500 bg-white border-2 border-gray-200 rounded-lg cursor-pointer dark:hover:text-gray-300 dark:border-gray-700 peer-checked:border-blue-600 dark:peer-checked:border-blue-600 hover:text-gray-600 dark:peer-checked:text-gray-300 peer-checked:text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:bg-gray-800 dark:hover:bg-gray-700",
		},
			elem.Div(attrs.Props{
				attrs.Class: "block",
			},
				elem.Img(attrs.Props{
					attrs.Src:   imageSrc,
					attrs.Alt:   label + " Logo",
					"data-js":   "option-label",
					attrs.Class: "w-14 h-14",
				}),
				elem.Div(attrs.Props{
					attrs.Class: "w-full text-sm",
				}, elem.Text(description)),
			),
		),
	)
}

// createVariantOption creates a variant option with the same styling as the original
func (bf *BuildForm) createVariantOption(id, label, imageSrc, description string, isSelected bool) *elem.Element {
	selected := bf.isSelected("variant", id) || (isSelected && bf.getSelectedValue("variant") == "")

	checkedAttr := ""
	if selected {
		checkedAttr = "checked"
	}

	return elem.Li(attrs.Props{},
		elem.Input(bf.mergeAttrs(attrs.Props{
			attrs.Type:     "radio",
			attrs.Name:     "variant",
			attrs.ID:       id + "-option",
			attrs.Value:    id,
			attrs.Class:    "hidden peer",
			attrs.Required: "true",
			"checked":      checkedAttr,
		}, bf.getHtmxAttrs("variant", "change")),
		),
		elem.Label(attrs.Props{
			attrs.For:   id + "-option",
			attrs.Class: "inline-flex items-center justify-between w-full p-3 text-gray-500 bg-white border-2 border-gray-200 rounded-lg cursor-pointer dark:hover:text-gray-300 dark:border-gray-700 peer-checked:border-blue-600 dark:peer-checked:border-blue-600 hover:text-gray-600 dark:peer-checked:text-gray-300 peer-checked:text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:bg-gray-800 dark:hover:bg-gray-700",
		},
			elem.Div(attrs.Props{
				attrs.Class: "block",
			},
				elem.Img(attrs.Props{
					attrs.Src:   imageSrc,
					attrs.Alt:   label + " Logo",
					"data-js":   "option-label",
					attrs.Class: "w-14 h-14",
				}),
				elem.Div(attrs.Props{
					attrs.Class: "w-full text-sm",
				}, elem.Text(description)),
			),
		),
	)
}

// createArtifactOption creates an artifact option with the same styling as the original
func (bf *BuildForm) createArtifactOption(id, label, imageSrc, secondImageSrc, description string, isSelected, isDisabled bool) *elem.Element {
	// For checkboxes, check if the value is "on" (checked) or if it's the default selected state
	selected := bf.isSelected(id, "on") || (isSelected && bf.getSelectedValue(id) == "")

	checkedAttr := ""
	if selected {
		checkedAttr = "checked"
	}

	disabledAttr := ""
	if isDisabled {
		disabledAttr = "disabled"
	}

	labelClass := "flex flex-col items-start justify-between w-full h-full p-6 text-gray-500 bg-white border-2 border-gray-200 rounded-lg cursor-pointer dark:border-gray-700 dark:text-gray-400 dark:bg-gray-800"
	if isDisabled {
		labelClass = "flex flex-col items-start justify-between w-full h-full p-6 text-gray-500 bg-white border-2 border-gray-200 rounded-lg cursor-not-allowed dark:border-gray-700 dark:text-gray-400 dark:bg-gray-800"
	}

	return elem.Li(attrs.Props{},
		elem.Input(bf.mergeAttrs(attrs.Props{
			attrs.Type:  "checkbox",
			attrs.ID:    id,
			attrs.Name:  id,
			attrs.Class: "hidden peer",
			"checked":   checkedAttr,
			"disabled":  disabledAttr,
		}, bf.getHtmxAttrs(id, "change")),
		),
		elem.Label(attrs.Props{
			attrs.For:   id,
			attrs.Class: labelClass,
		},
			elem.Div(attrs.Props{
				attrs.Class: "block",
			},
				elem.Span(attrs.Props{
					attrs.Class: "text-gray-900 dark:text-white font-semibold text-right flex flex-row gap-2 items-center",
					"aria-live": "polite",
				},
					elem.Img(attrs.Props{
						attrs.Src:   imageSrc,
						attrs.Alt:   label,
						attrs.Class: "mb-2 w-7 h-7",
					}),
					func() *elem.Element {
						if secondImageSrc != "" {
							return elem.Img(attrs.Props{
								attrs.Src:   secondImageSrc,
								attrs.Alt:   "AWS Logo",
								attrs.Class: "w-7 h-7",
							})
						}
						return elem.Span(attrs.Props{})
					}(),
				),
				elem.Div(attrs.Props{
					attrs.Class: "w-full text-lg font-bold mb-2",
				}, elem.Text(label)),
				elem.Div(attrs.Props{
					attrs.Class: "w-full text-sm",
				}, elem.Text(description)),
			),
		),
	)
}

// createAccordionIcon creates the accordion arrow icon
func (bf *BuildForm) createAccordionIcon() *elem.Element {
	return elem.Div(attrs.Props{
		"data-accordion-icon": "",
		attrs.Class:           "w-3 h-3 rotate-180 shrink-0",
		"aria-hidden":         "true",
	},
		elem.Text("▼"),
	)
}

// getSelectedValue retrieves the value for a given field name
func (bf *BuildForm) getSelectedValue(fieldName string) string {
	switch fieldName {
	case "base_image":
		return bf.BaseImage
	case "architecture":
		return bf.Architecture
	case "model":
		return bf.Model
	case "variant":
		return bf.Variant
	case "version":
		return bf.Version
	case "cloud_config":
		return bf.CloudConfig
	case "artifact_raw":
		return bf.ArtifactRaw
	case "artifact_iso":
		return bf.ArtifactISO
	case "artifact_tar":
		return bf.ArtifactTAR
	default:
		return ""
	}
}

// isSelected checks if an option should be selected
func (bf *BuildForm) isSelected(fieldName, optionValue string) bool {
	return bf.getSelectedValue(fieldName) == optionValue
}

// getHtmxAttrs generates HTMX attributes for form fields
func (bf *BuildForm) getHtmxAttrs(changedField string, trigger string) attrs.Props {
	htmxAttrs := attrs.Props{
		"hx-post": "/changed-option",
		// TODO: This doesn't render correctly
		"hx-vals":      fmt.Sprintf(`{"changed": "%s"}`, changedField),
		"hx-include":   "[id='build-form']",
		"hx-indicator": ".htmx-indicator",
		"hx-target":    "#build-form",
	}
	if trigger != "" {
		htmxAttrs["hx-trigger"] = trigger
	}
	return htmxAttrs
}

// mergeAttrs merges two attrs.Props maps
func (bf *BuildForm) mergeAttrs(base, additional attrs.Props) attrs.Props {
	result := make(attrs.Props)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range additional {
		result[k] = v
	}
	return result
}
