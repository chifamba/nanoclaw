package container

import (
	"fmt"
)

// Container runtime constants and helper functions

// ContainerRuntimeBin is the container runtime binary name.
var ContainerRuntimeBin = "container"

// ContainerHostGateway is the hostname containers use to reach the host machine.
const ContainerHostGateway = "192.168.64.1"

// ProxyBindHost is the address the credential proxy binds to.
const ProxyBindHost = "0.0.0.0"

// HostGatewayArgs returns CLI args needed for the container to resolve the host gateway.
func HostGatewayArgs() []string {
	return []string{}
}

// ReadonlyMountArgs returns CLI args for a readonly bind mount.
func ReadonlyMountArgs(hostPath, containerPath string) []string {
	return []string{"-v", fmt.Sprintf("%s:%s:ro", hostPath, containerPath)}
}

// StopContainer returns the shell command to stop a container by name.
func StopContainer(name string) string {
	return fmt.Sprintf("%s stop %s", ContainerRuntimeBin, name)
}
