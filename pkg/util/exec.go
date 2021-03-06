package util

/*

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

import (
	"bytes"
	"io"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// https://github.com/kubernetes/kubernetes/blob/v1.6.1/test/e2e/framework/exec_util.go
// Global variables
var (
	clientset      *kubernetes.Clientset
	restConfig     *restclient.Config
	log            logr.Logger = ctrl.Log.WithName("kubeclient")
	kubeconfigPath             = "~/.kube/config"
)

// ExecOptions passed to ExecWithOptions
type ExecOptions struct {
	Command []string

	Namespace     string
	PodName       string
	ContainerName string

	Stdin         io.Reader
	CaptureStdout bool
	CaptureStderr bool
	// If false, whitespace in std{err,out} will be removed.
	PreserveWhitespace bool
}

func initClient() error {
	var err error

	if restConfig == nil {

		home, err := Home()
		if err != nil {
			return err
		}
		kubeconfigPath = path.Join(home, ".kube/config")
		if len(os.Getenv("KUBECONFIG")) > 0 {
			kubeconfigPath = os.Getenv("KUBECONFIG")
		}
		if !PathExists(kubeconfigPath) {
			kubeconfigPath = ""
		}
		log.Info("kubeconfig file is placed.", "config", kubeconfigPath)
		restConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return err
		}
	}
	if clientset == nil {
		clientset, err = kubernetes.NewForConfig(restConfig)
		if err != nil {
			return err
		}
	}
	return nil
}

// ExecWithOptions executes a command in the specified container,
// returning stdout, stderr and error. `options` allowed for
// additional parameters to be passed.
func ExecWithOptions(options ExecOptions) (string, string, error) {
	err := initClient()
	if err != nil {
		return "", "", err
	}

	log.V(1).Info("ExecWithOptions", "ExecWithOptions", options)

	const tty = false

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(options.PodName).
		Namespace(options.Namespace).
		SubResource("exec").
		Param("container", options.ContainerName)
	req.VersionedParams(&v1.PodExecOptions{
		Container: options.ContainerName,
		Command:   options.Command,
		Stdin:     options.Stdin != nil,
		Stdout:    options.CaptureStdout,
		Stderr:    options.CaptureStderr,
		TTY:       tty,
	}, scheme.ParameterCodec)

	var stdout, stderr bytes.Buffer
	err = execute("POST", req.URL(), restConfig, options.Stdin, &stdout, &stderr, tty)

	if options.PreserveWhitespace {
		return stdout.String(), stderr.String(), err
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// ExecCommandInContainerWithFullOutput executes a command in the
// specified container and return stdout, stderr and error
func ExecCommandInContainerWithFullOutput(podName string, containerName string, namespace string, cmd []string) (stdout string, stderr string, err error) {
	return ExecWithOptions(ExecOptions{
		Command:       cmd,
		Namespace:     namespace,
		PodName:       podName,
		ContainerName: containerName,

		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: false,
	})
}

func ExecShellInContainer(podName string, containerName string, namespace string, cmd string) (stdout string, stderr string, err error) {
	return ExecCommandInContainer(podName, containerName, namespace, []string{"/bin/sh", "-c", cmd})
}

func ExecCommandInContainer(podName string, containerName string, namespace string, cmd []string) (stdout string, stderr string, err error) {
	return ExecCommandInContainerWithFullOutput(podName, containerName, namespace, cmd)
}

func ExecCommandInPod(pod *corev1.Pod, cmd []string) (stdout string, stderr string, err error) {
	return ExecCommandInContainer(pod.Name, pod.Spec.Containers[0].Name, pod.Namespace, cmd)
}

func execute(method string, url *url.URL, config *restclient.Config, stdin io.Reader, stdout, stderr io.Writer, tty bool) error {
	exec, err := remotecommand.NewSPDYExecutor(config, method, url)
	if err != nil {
		return err
	}
	return exec.Stream(remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    tty,
	})
}

func PathExists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}
