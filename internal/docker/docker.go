package docker

import (
	"os/exec"
	"strings"
)

type Container struct {
	Name   string
	Image  string
	Status string
	Ports  string
}

func ListContainers() []Container {
	out, err := exec.Command("docker", "ps", "--format", "{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}").Output()
	if err != nil {
		return nil
	}

	var containers []Container
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 3 {
			continue
		}

		c := Container{
			Name:   parts[0],
			Image:  parts[1],
			Status: parts[2],
		}
		if len(parts) == 4 {
			c.Ports = simplifyPorts(parts[3])
		}
		containers = append(containers, c)
	}

	return containers
}

// simplifyPorts extracts just the host:port mappings
func simplifyPorts(raw string) string {
	if raw == "" {
		return ""
	}
	var ports []string
	for _, mapping := range strings.Split(raw, ", ") {
		// "0.0.0.0:8095->80/tcp" => ":8095->80"
		if idx := strings.Index(mapping, "->"); idx >= 0 {
			host := mapping[:idx]
			container := mapping[idx+2:]
			// Strip protocol
			container = strings.TrimSuffix(container, "/tcp")
			container = strings.TrimSuffix(container, "/udp")
			// Strip 0.0.0.0 and [::]
			host = strings.TrimPrefix(host, "0.0.0.0")
			host = strings.TrimPrefix(host, "[::]")
			// Deduplicate (ipv4 and ipv6 bindings)
			port := host + "->" + container
			found := false
			for _, p := range ports {
				if p == port {
					found = true
					break
				}
			}
			if !found {
				ports = append(ports, port)
			}
		}
	}
	return strings.Join(ports, " ")
}
