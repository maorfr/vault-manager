// Package audit implements the application of a declarative configuration
// for Vault Audit Devices.
package audit

import (
	"github.com/hashicorp/vault/api"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"github.com/app-sre/vault-manager/pkg/vault"
	"github.com/app-sre/vault-manager/toplevel"
)

type entry struct {
	Path        string            `yaml:"_path"`
	Type        string            `yaml:"type"`
	Description string            `yaml:"description"`
	Options     map[string]string `yaml:"options"`
}

var _ vault.Item = entry{}

func (e entry) Key() string {
	return e.Path
}

func (e entry) Equals(i interface{}) bool {
	entry, ok := i.(entry)
	if !ok {
		return false
	}

	return vault.EqualPathNames(e.Path, entry.Path) &&
		e.Type == entry.Type &&
		e.Description == entry.Description &&
		vault.OptionsEqual(e.ambiguousOptions(), entry.ambiguousOptions())
}

func (e entry) ambiguousOptions() map[string]interface{} {
	opts := make(map[string]interface{}, len(e.Options))
	for k, v := range e.Options {
		opts[k] = v
	}
	return opts
}

func (e entry) enable(client *api.Client) {
	if err := client.Sys().EnableAuditWithOptions(e.Path, &api.EnableAuditOptions{
		Type:        e.Type,
		Description: e.Description,
		Options:     e.Options,
	}); err != nil {
		logrus.WithField("path", e.Path).Fatal("failed to enable audit device")
	}
	logrus.WithField("path", e.Path).Info("audit successfully enabled")
}

func (e entry) disable(client *api.Client) {
	if err := client.Sys().DisableAudit(e.Path); err != nil {
		logrus.WithField("path", e.Path).Fatal("failed to disable audit")
	}
	logrus.WithField("path", e.Path).Info("audit successfully disabled")
}

type config struct{}

var _ toplevel.Configuration = config{}

func init() {
	toplevel.RegisterConfiguration("vault_audit_backends", config{})
}

// Apply ensures that an instance of Vault's Audit Devices are configured
// exactly as provided.
//
// This function exits the program if an error occurs.
func (c config) Apply(entriesBytes []byte, dryRun bool) {
	var entries []entry
	if err := yaml.Unmarshal(entriesBytes, &entries); err != nil {
		logrus.WithError(err).Fatal("failed to decode Audit Devices configuration")
	}

	// Get the existing enabled Audits Devices.
	enabledAudits, err := vault.ClientFromEnv().Sys().ListAudit()
	if err != nil {
		logrus.WithError(err).Fatal("failed to list Audit Devices from Vault instance")
	}

	// Build a list of all the existing entries.
	existingAudits := make([]entry, 0)
	if enabledAudits != nil {
		for _, audit := range enabledAudits {
			existingAudits = append(existingAudits, entry{
				Path:        audit.Path,
				Type:        audit.Type,
				Description: audit.Description,
				Options:     audit.Options,
			})
		}
	}

	// Diff the local configuration with the Vault instance.
	toBeWritten, toBeDeleted := vault.DiffItems(asItems(entries), asItems(existingAudits))

	if dryRun == true {
		for _, w := range toBeWritten {
			logrus.Infof("[Dry Run]\tpackage=audit\tentry to be written='%v'", w)
		}
		for _, d := range toBeDeleted {
			logrus.Infof("[Dry Run]\tpackage=audit\tentry to be deleted='%v'", d)
		}
	} else {
		// Write any missing Audit Devices to the Vault instance.
		for _, e := range toBeWritten {
			e.(entry).enable(vault.ClientFromEnv())
		}

		// Delete any Audit Devices from the Vault instance.
		for _, e := range toBeDeleted {
			e.(entry).disable(vault.ClientFromEnv())
		}
	}
}

func asItems(xs []entry) (items []vault.Item) {
	items = make([]vault.Item, 0)
	for _, x := range xs {
		items = append(items, x)
	}

	return
}
