package VMNodeGolang

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"github.com/fsouza/go-dockerclient"
	"time"
)

const (
	letterBytes   = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	letterIdxBits = 6
	letterIdxMask = 1<<letterIdxBits - 1
)

type (
	VMResult struct {
		ErrOut bytes.Buffer
		LogOut bytes.Buffer
	}

	VMOptions struct {
		Timeout uint
		Binds   []string
	}

	VM struct {
		dockerClient *docker.Client
		image        string
	}
)

func NewVM(image string) (vm *VM, err error) {
	vm = &VM{
		image: "aardvarkx1/scnode:latest",
	}

	if len(image) > 0 {
		vm.image = image
	}

	endpoint := "unix:///var/run/docker.sock"
	vm.dockerClient, err = docker.NewClient(endpoint)
	if err != nil {
		return
	}

	return
}

func (vm *VM) randomBytes(length int) []byte {
	var randomBytes = make([]byte, length)
	_, err := rand.Read(randomBytes)
	if err != nil {
		panic("Unable to generate random bytes")
	}
	return randomBytes
}

func (vm *VM) randomString(length int) string {
	result := make([]byte, length)
	bufferSize := int(float64(length) * 1.3)
	for i, j, randomBytes := 0, 0, []byte{}; i < length; j++ {
		if j%bufferSize == 0 {
			randomBytes = vm.randomBytes(bufferSize)
		}
		if idx := int(randomBytes[j%length] & letterIdxMask); idx < len(letterBytes) {
			result[i] = letterBytes[idx]
			i++
		}
	}

	return string(result)
}

func (vm *VM) Run(script string, opt VMOptions) (result *VMResult, err error) {
	result = &VMResult{}

	dk_conf := docker.Config{
		Image:    vm.image,
		User:     "app",
		Cmd:      []string{"node", "-e", script},
	}

	dk_hostconf := docker.HostConfig{
		PidsLimit: 1000,
		Memory:    209715200,
		Binds:     opt.Binds,
	}

	container, err := vm.dockerClient.CreateContainer(docker.CreateContainerOptions{
		Name:       vm.randomString(20),
		Config:     &dk_conf,
		HostConfig: &dk_hostconf,
	})
	if err != nil {
		return
	}

	err = vm.dockerClient.StartContainer(container.ID, nil)
	if err != nil {
		return
	}

	finish := make(chan bool)
	timer := time.NewTimer(time.Second * 2)

	go func() {
		vm.dockerClient.WaitContainer(container.ID)
		close(finish)
	}()

	defer func() {
		timer.Stop()
		logWriter := bufio.NewWriter(&result.LogOut)
		errWriter := bufio.NewWriter(&result.ErrOut)

		logOpts := docker.LogsOptions{
			Container:    container.ID,
			OutputStream: logWriter,
			ErrorStream:  errWriter,
			Follow:       false,
			Stdout:       true,
			Stderr:       true,
		}
		vm.dockerClient.Logs(logOpts)

		logWriter.Flush()
		errWriter.Flush()

		rmOpts := docker.RemoveContainerOptions{
			ID:            container.ID,
			RemoveVolumes: false,
			Force:         true,
		}
		err = vm.dockerClient.RemoveContainer(rmOpts)
	}()

	select {
	case <- finish:
		break
	case <- timer.C:
		err = vm.dockerClient.StopContainer(container.ID, 1)
		if err != nil {
			return
		}
	}

	return
}