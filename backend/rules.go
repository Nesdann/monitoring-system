package main

import (
	"fmt"
	"monitoring-system/protocol"
)

func evaluateRules(event protocol.Event, state *AgentState) {
	switch event.Type {
	case "metrics":
		ruleCPUSostenida(event, state)

	case "process":
		procs, ok := event.Data["processes"].([]any)
		if !ok {
			return
		}
		for _, p := range procs {
			proc, ok := p.(map[string]any)
			if !ok {
				continue
			}
			ruleProcesoRootDesconocido(event, proc, state)
			ruleProcesoNuevo(event, proc, state)
		}

	case "connection_snapshot":
		conns, ok := event.Data["connections"].([]any)
		if !ok {
			return
		}
		for _, c := range conns {
			conn, ok := c.(map[string]any)
			if !ok {
				continue
			}
			ruleConexionPuertoRaro(event, conn)
			ruleConexionNueva(event, conn, state)
		}
	}
}

func ruleCPUSostenida(event protocol.Event, state *AgentState) {
	cpu := toFloat(event.Data["cpu"])
	state.RecentCPUs = append(state.RecentCPUs, cpu)
	if len(state.RecentCPUs) > 3 {
		state.RecentCPUs = state.RecentCPUs[1:] // mantiene solo los últimos 3
	}
	if len(state.RecentCPUs) < 3 {
		return
	}
	for _, c := range state.RecentCPUs {
		if c < 80.0 {
			return
		}
	}
	fmt.Printf("[ALERTA] %s  CPU alta sostenida: %.1f %.1f %.1f\n",
		event.Hostname,
		state.RecentCPUs[0],
		state.RecentCPUs[1],
		state.RecentCPUs[2],
	)
}

func ruleProcesoRootDesconocido(event protocol.Event, proc map[string]any, state *AgentState) {
	username, _ := proc["user"].(string)
	name, _ := proc["name"].(string)
	if username != "root" {
		return
	}
	knownRoot := map[string]bool{
		"systemd": true, "systemd-journald": true, "systemd-udevd": true,
		"systemd-logind": true, "NetworkManager": true, "snapd": true,
		"dockerd": true, "containerd": true, "apache2": true,
		"accounts-daemon": true, "udisksd": true, "polkitd": true,
		"power-profiles-daemon": true, "switcheroo-control": true,
		"wpa_supplicant": true, "cron": true, "rsyslogd": true,
		"containerd-shim-runc-v2": true, "mysqld": true, "lightdm": true,
		"Xorg": true, "cupsd": true, "cups-browsed": true, "fwupd": true,
		"ModemManager": true, "upowerd": true, "kerneloops": true,
		"agetty": true, "docker-proxy": true, "psimon": true,
	}
	if knownRoot[name] || isKernelProcess(name) {
		return
	}
	key := "root:" + name
	if !state.KnownProcesses[key] {
		state.KnownProcesses[key] = true
		fmt.Printf("[ALERTA] %s  proceso root desconocido: %s (pid %v)\n",
			event.Hostname, name, proc["pid"])
	}
}

func ruleProcesoNuevo(event protocol.Event, proc map[string]any, state *AgentState) {
	name, _ := proc["name"].(string)
	if !state.KnownProcesses[name] {
		state.KnownProcesses[name] = true
		// solo alerta si ya pasó el primer snapshot (warm-up)
		if len(state.KnownProcesses) > 50 {
			fmt.Printf("[ALERTA] %s  proceso nuevo detectado: %s (pid %v)\n",
				event.Hostname, name, proc["pid"])
		}
	}
}

func ruleConexionPuertoRaro(event protocol.Event, conn map[string]any) {
	dstPort := toFloat(conn["dst_port"])
	dstIP, _ := conn["dst_ip"].(string)
	srcIP, _ := conn["src_ip"].(string)
	status, _ := conn["status"].(string)

	if dstIP == "" || dstIP == "0.0.0.0" || dstIP == "::" {
		return
	}
	if status != "ESTABLISHED" {
		return
	}
	// ignorar tráfico interno localhost
	if dstIP == "127.0.0.1" && srcIP == "127.0.0.1" {
		return
	}

	knownPorts := map[float64]bool{
		80: true, 443: true, 53: true, 22: true,
	}
	if !knownPorts[dstPort] {
		fmt.Printf("[ALERTA] %s  conexion a puerto raro: %s:%v (pid %v)\n",
			event.Hostname, dstIP, dstPort, conn["pid"])
	}
}

func isKernelProcess(name string) bool {
	prefixes := []string{
		"kworker", "kthread", "ksoftirqd", "migration",
		"rcu_", "idle_inject", "cpuhp", "pool_workqueue",
		"kswapd", "khugepaged", "ksmd",
	}
	for _, p := range prefixes {
		if len(name) >= len(p) && name[:len(p)] == p {
			return true
		}
	}
	return false
}

func ruleConexionNueva(event protocol.Event, conn map[string]any, state *AgentState) {
	dstIP, _ := conn["dst_ip"].(string)
	dstPort := toFloat(conn["dst_port"])
	status, _ := conn["status"].(string)

	if dstIP == "" || status != "ESTABLISHED" {
		return
	}

	key := fmt.Sprintf("%s:%v", dstIP, dstPort)
	if !state.KnownConnections[key] {
		state.KnownConnections[key] = true
		fmt.Printf("[ALERTA] %s  conexion nueva: %s (pid %v)\n",
			event.Hostname, key, conn["pid"])
	}
}
