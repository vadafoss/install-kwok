package main

import (
	"bytes"
	"context"
	"fmt"
	"install-kwok/pkg/constants"
	"net/http"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/mod/semver"

	"github.com/google/go-github/v53/github"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	kubectlcmd "k8s.io/kubectl/pkg/cmd"
)

var installRelease string

func init() {
	os.Setenv("POD_NAMESPACE", "default")
	os.Setenv("SERVICE_ACCOUNT", "cluster-autoscaler")
}

func main() {

	// Check and load kubeconfig from the path set
	// in KUBECONFIG env variable (if not use default path of ~/.kube/config)
	apiConfig, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		panic(err)
	}

	// Create rest config from kubeconfig
	restConfig, err := clientcmd.NewDefaultClientConfig(*apiConfig, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		panic(err)
	}

	kubeClient := kubeclient.NewForConfigOrDie(restConfig)

	rel, err := GetLatestKwokRelease()
	if err != nil {
		panic(err)
	}

	c := semver.Compare(rel, constants.MinVersion)
	if c < 0 {
		log.Fatalf("latest release %s is a lower version than min required version %s\n", rel, constants.MinVersion)
	}

	installRelease = rel

	// if err := DeleteClusterRoleBinding(kubeClient); !errors.IsNotFound(err) {
	// 	log.Infof("failed deleting existing `kwok-provider` ClusterRoleBinding: %v", err)
	// }

	// if err := UninstallKwokIgnorePanic(); err != nil {
	// 	log.Infof("err: %v", err)
	// }

	if err := InstallClusterRoleBinding(kubeClient); err != nil {
		panic(err)
	}

	if err := InstallKwok(); err != nil {
		panic(err)
	}

	time.Sleep(time.Second * 20)
	if err := UninstallKwok(); err != nil {
		panic(err)
	}

	if err := DeleteClusterRoleBinding(kubeClient); err != nil {
		panic(err)
	}

}

// InstallClusterRoleBinding creates a ClusterRoleBinding between
// `cluster-admin` ClusterRole and cluster-autoscaler's ServiceAccount
func InstallClusterRoleBinding(kubeClient kubernetes.Interface) error {
	ns := os.Getenv("POD_NAMESPACE")
	sa := os.Getenv("SERVICE_ACCOUNT")
	crb := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "kwok-provider"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{
			{Kind: rbacv1.ServiceAccountKind, Namespace: ns, Name: sa},
		},
	}
	if _, err := kubeClient.RbacV1().ClusterRoleBindings().
		Create(context.Background(), &crb, metav1.CreateOptions{}); err != nil {
		return err
	}
	return nil
}

// DeleteClusterRoleBinding deletes the ClusterRoleBinding between
// `cluster-admin` ClusterRole and cluster-autoscaler's ServiceAccount
func DeleteClusterRoleBinding(kubeClient kubernetes.Interface) error {
	if err := kubeClient.RbacV1().ClusterRoleBindings().
		Delete(context.Background(), constants.CrbName, metav1.DeleteOptions{}); !errors.IsNotFound(err) {
		return err
	}

	return nil
}

// InstallKwok installs kwok >= v0.4.0
// Based on https://kwok.sigs.k8s.io/docs/user/kwok-in-cluster/#deploy-kwok-in-a-cluster
func InstallKwok() error {
	deploymentAndCRDsURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/kwok.yaml",
		constants.KwokRepo,
		installRelease)
	if err := kwokKubectl(deploymentAndCRDsURL, constants.Apply, nil); err != nil {
		return err
	}

	stagesCRs := fmt.Sprintf("https://github.com/%s/releases/download/%s/stage-fast.yaml",
		constants.KwokRepo,
		installRelease)
	return kwokKubectl(stagesCRs, constants.Apply, nil)
}

// UninstallKwok uninstalls kwok >= v0.4.0
// Based on https://kwok.sigs.k8s.io/docs/user/kwok-in-cluster/#deploy-kwok-in-a-cluster
func UninstallKwok() error {
	stagesCRs := fmt.Sprintf("https://github.com/%s/releases/download/%s/stage-fast.yaml",
		constants.KwokRepo,
		installRelease)
	if err := kwokKubectl(stagesCRs, constants.Delete, []string{"--ignore-not-found=true"}); err != nil {
		return err
	}

	deploymentAndCRDsURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/kwok.yaml",
		constants.KwokRepo,
		installRelease)
	return kwokKubectl(deploymentAndCRDsURL, constants.Delete, []string{"--ignore-not-found=true"})
}

func kwokKubectl(url string, action constants.Action, extraArgs []string) error {

	// `kubectl apply` deployment and CRDs
	cmd := []string{
		"kubectl", string(action), "-f", url,
	}
	if len(extraArgs) > 0 {
		cmd = append(cmd, extraArgs...)
	}
	o, err := runKubectl(cmd...)
	if err != nil {
		return err
	}
	log.Infof("kubectl output: \n%s", string(o))

	return nil
}

func GetLatestKwokRelease() (string, error) {
	// find latest release of `kwok`
	client := github.NewClient(nil)
	rel, resp, err := client.Repositories.GetLatestRelease(context.Background(), constants.Owner, constants.Repo)
	if err != nil {
		return "", err
	}

	if resp.Response.StatusCode != http.StatusOK {
		log.Fatal("expected 200 response but received", resp.Response.StatusCode)
	}

	return rel.GetTagName(), nil
}

// based on argo-workflows executor
// code ref: https://github.com/argoproj/argo-workflows/blob/545bf3803d6f0c59a4c0a93db23d18001462bf3c/workflow/executor/resource.go#L366
func runKubectl(args ...string) ([]byte, error) {
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
