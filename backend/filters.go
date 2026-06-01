package main

func isInterestingProcess(proc map[string]any) bool {
    cpu := toFloat(proc["cpu"])
    mem := toFloat(proc["mem"])
    username, _ := proc["user"].(string)
    name, _ := proc["name"].(string)

    // cualquier proceso con CPU alta es interesante
    if cpu > 5.0 {
        return true
    }

    // procesos root que no son kernel y tienen memoria real
    if username == "root" {
        kernelPrefixes := []string{
            "kworker", "kthread", "ksoftirqd", "migration",
            "rcu_", "idle_inject", "cpuhp", "pool_workqueue",
            "kswapd", "khugepaged", "ksmd", "kdevtmpfs",
            "kauditd", "khungtaskd", "oom_reaper", "kcompactd",
            "irq/", "watchdogd", "ecryptfs", "scsi_",
        }
        for _, prefix := range kernelPrefixes {
            if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
                return false
            }
        }
        // root, no es kernel, y tiene memoria real → interesante
        if mem > 0.05 {
            return true
        }
    }

    return false
}

func isInterestingConnection(conn map[string]any) bool {
    dstPort := toFloat(conn["dst_port"])
    status, _ := conn["status"].(string)

    if dstPort != 443 && dstPort != 80 && dstPort != 0 {
        return true
    }
    if status == "TIME_WAIT" {
        return true
    }
    return false
}