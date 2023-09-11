package legacy

import (
	"bytes"
	"fmt"
	"html/template"
	"install-kwok/pkg/constants"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"golang.org/x/mod/semver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	kubectlcmd "k8s.io/kubectl/pkg/cmd"
)

// kwokKubectlLegacy builds kustomize for kwok < v0.4.0 and runs `kubectl` on it
func kwokKubectlLegacy(release string, action constants.Action) error {

	if release == "" {
		return fmt.Errorf("release is empty: '%s'", release)
	}

	// create tmp working directory for kwok
	tmpDir, err := ioutil.TempDir("", "install-kwok")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// create kustomization file
	kustomizeTemplate := `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
images:
- name: registry.k8s.io/kwok/kwok
  newTag: "{{.latestRelease}}"
resources:
- "https://github.com/{{.repo}}/kustomize/kwok?ref={{.latestRelease}}"
`
	tmpl, err := template.New("kwok-kustomize").Parse(kustomizeTemplate)
	if err != nil {
		return err
	}

	b := &bytes.Buffer{}
	err = tmpl.Execute(b, map[string]interface{}{
		"latestRelease": release,
		"repo":          constants.KwokRepo,
	})

	if err != nil {
		return err
	}

	log.Infof("latest kwok release is %s", release)

	kustomizationFilePath := filepath.Join(tmpDir, "kustomization.yaml")
	if err := os.WriteFile(kustomizationFilePath,
		b.Bytes(),
		os.FileMode(0600)); err != nil {
		log.Fatal(err)
	}

	// `kubectl apply` using kustomize
	o, err := runKubectlLegacy("kubectl", string(action), "-k", tmpDir)
	if err != nil {
		return err
	}
	log.Infof("kubectl output: \n%s", string(o))

	return nil
}

// based on argo-workflows executor
// code ref: https://github.com/argoproj/argo-workflows/blob/545bf3803d6f0c59a4c0a93db23d18001462bf3c/workflow/executor/resource.go#L366
func runKubectlLegacy(args ...string) ([]byte, error) {
	log.Info(strings.Join(args, " "))
	os.Args = args
	var buf bytes.Buffer
	if err := kubectlcmd.NewKubectlCommand(kubectlcmd.KubectlOptions{
		Arguments: args,
		ConfigFlags: genericclioptions.NewConfigFlags(true).
			WithDeprecatedPasswordFlag().
			WithDiscoveryBurst(300).
			WithDiscoveryQPS(50.0),
		IOStreams: genericclioptions.IOStreams{Out: &buf, ErrOut: os.Stderr},
	}).Execute(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// InstallKwokLegacy installs kwok < v0.4.0 in the cluster
func InstallKwokLegacy(release string) error {
	return kwokKubectlLegacy(release, constants.Apply)
}

// LegacyInstallAndUninstall installs and uninstalls kwok the legacy way
func LegacyInstallAndUninstall(release string) {
	c := semver.Compare(release, constants.MinVersion)
	if c > 0 {
		log.Fatalf("release %s is a version higher than version %s\n", release, constants.MinVersion)
	}

	if err := InstallKwokLegacy(release); err != nil {
		panic(err)
	}

	time.Sleep(20 * time.Second)
	if err := UninstallKwokLegacy(release); err != nil {
		panic(err)
	}
}

// UninstallKwokLegacy uninstalls kwok < v0.4.0 from the cluster
func UninstallKwokLegacy(release string) error {
	return kwokKubectlLegacy(release, constants.Delete)
}
