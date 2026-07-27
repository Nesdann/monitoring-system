use tokio_postgres::Client;
use anyhow::Result;
use std::time::{SystemTime, UNIX_EPOCH};
use crate::detectors::zscore::ZScoreDetector;
use crate::detectors::moving_average::MovingAverageDetector;
use crate::detectors::Detector;
use crate::alerts::save_alert;
use crate::detectors::ewma::EwmaDetector;
use crate::detectors::mad::MadDetector;

pub async fn get_hosts(client: &Client) -> Result<Vec<String>> {
    let rows = client
        .query("SELECT DISTINCT hostname FROM metrics", &[])
        .await?;
    Ok(rows.into_iter().map(|row| row.get(0)).collect())
}

pub async fn get_cpu_samples(client: &Client, hostname: &str) -> Result<Vec<f64>> {
    let rows = client
        .query(
            "SELECT cpu FROM metrics WHERE hostname = $1 ORDER BY timestamp DESC LIMIT 30",
            &[&hostname],
        )
        .await?;
    Ok(rows.into_iter().map(|row| row.get(0)).collect())
}

pub async fn get_connection_samples(client: &Client, hostname: &str) -> Result<Vec<f64>> {
    let rows = client
        .query(
            "
            SELECT COUNT(*)::float8 AS cnt
            FROM connections
            WHERE hostname = $1
            GROUP BY timestamp
            ORDER BY timestamp DESC
            LIMIT 30
            ",
            &[&hostname],
        )
        .await?;
    Ok(rows.into_iter().map(|row| row.get(0)).collect())
}

// NEW: list of actual process names seen for this host, so we know what to loop over
pub async fn get_process_names(client: &Client, hostname: &str) -> Result<Vec<String>> {
    let rows = client
        .query(
            "SELECT DISTINCT name FROM processes WHERE hostname = $1",
            &[&hostname],
        )
        .await?;
    Ok(rows.into_iter().map(|row| row.get(0)).collect())
}

pub async fn get_process_samples(
    client: &Client,
    hostname: &str,
    name: &str,
    metric: &str, // "cpu" or "mem"
) -> Result<Vec<f64>> {
    let column = match metric {
        "cpu" => "cpu",
        "mem" => "mem",
        _ => return Err(anyhow::anyhow!("invalid metric: {}", metric)),
    };

    let query = format!(
        "SELECT {column} FROM processes WHERE hostname = $1 AND name = $2 ORDER BY timestamp DESC LIMIT 30"
    );

    let rows = client.query(&query, &[&hostname, &name]).await?;
    Ok(rows.into_iter().map(|row| row.get(0)).collect())
}

// analyze_host: cpu + connection-count series only, one series per host
pub async fn analyze_host(client: &Client, hostname: &str) -> Result<()> {
    println!("Analyzing host take samples: {}", hostname);

    let samples_cpu = get_cpu_samples(client, hostname).await?;
    let conn_counts = get_connection_samples(client, hostname).await?;

    if samples_cpu.is_empty() {
        println!("No samples for host CPU: {}", hostname);
    }
    if conn_counts.is_empty() {
        println!("No samples for host Connections: {}", hostname);
    }

    let zcoredetector = ZScoreDetector { threshold: 2.5 };
    if let Some(mut alert) = zcoredetector.analyze(hostname, &samples_cpu) {
        alert.category = "cpu".to_string();
        save_alert(client, &alert).await?;
    }
    if let Some(mut alert) = zcoredetector.analyze(hostname, &conn_counts) {
        alert.category = "connections".to_string();
        save_alert(client, &alert).await?;
    }

    let moving_avg_detector = MovingAverageDetector { deviation_threshold: 0.3, window_size: 10 };
    if let Some(mut alert) = moving_avg_detector.analyze(hostname, &samples_cpu) {
        alert.category = "cpu".to_string();
        save_alert(client, &alert).await?;
    }
    if let Some(mut alert) = moving_avg_detector.analyze(hostname, &conn_counts) {
        alert.category = "connections".to_string();
        save_alert(client, &alert).await?;
    }

    let ewma_detector = EwmaDetector { deviation_threshold: 0.3, alpha: 0.5 };
    if let Some(mut alert) = ewma_detector.analyze(hostname, &samples_cpu) {
        alert.category = "cpu".to_string();
        save_alert(client, &alert).await?;
    }
    if let Some(mut alert) = ewma_detector.analyze(hostname, &conn_counts) {
        alert.category = "connections".to_string();
        save_alert(client, &alert).await?;
    }

    let mad_detector = MadDetector { threshold: 2.5 };
    if let Some(mut alert) = mad_detector.analyze(hostname, &samples_cpu) {
        alert.category = "cpu".to_string();
        save_alert(client, &alert).await?;
    }
    if let Some(mut alert) = mad_detector.analyze(hostname, &conn_counts) {
        alert.category = "connections".to_string();
        save_alert(client, &alert).await?;
    }

    Ok(())
}


pub async fn analyze_processes(client: &Client, hostname: &str) -> Result<()> {
    let names = get_process_names(client, hostname).await?;

    let zcoredetector = ZScoreDetector { threshold: 2.5 };
    let moving_avg_detector = MovingAverageDetector { deviation_threshold: 0.3, window_size: 10 };
    let ewma_detector = EwmaDetector { deviation_threshold: 0.3, alpha: 0.5 };
    let mad_detector = MadDetector { threshold: 2.5 };

    for name in names {
        let cpu_samples = get_process_samples(client, hostname, &name, "cpu").await?;
        let mem_samples = get_process_samples(client, hostname, &name, "mem").await?;

        if cpu_samples.len() < 5 && mem_samples.len() < 5 {
            continue; // not enough history for this process yet
        }

        let label = format!("{hostname}:{name}");

        for samples in [&cpu_samples, &mem_samples] {
            if let Some(mut alert) = zcoredetector.analyze(&label, samples) {
                alert.category = "process".to_string();
                save_alert(client, &alert).await?;
            }
            if let Some(mut alert) = moving_avg_detector.analyze(&label, samples) {
                alert.category = "process".to_string();
                save_alert(client, &alert).await?;
            }
            if let Some(mut alert) = ewma_detector.analyze(&label, samples) {
                alert.category = "process".to_string();
                save_alert(client, &alert).await?;
            }
            if let Some(mut alert) = mad_detector.analyze(&label, samples) {
                alert.category = "process".to_string();
                save_alert(client, &alert).await?;
            }
        }
    }

    Ok(())
}

pub async fn analyze_process_relationships(client: &Client, hostname: &str) -> Result<()> {
    let now = SystemTime::now().duration_since(UNIX_EPOCH)?.as_secs() as i64;
    let window_start = now - 60; // only look at processes seen in the last minute

    // latest snapshot per (name, pid) within that window
    let rows = client
        .query(
            "
            SELECT DISTINCT ON (name, pid) name, pid, ppid, exe, cpu, create_time, timestamp
            FROM processes
            WHERE hostname = $1 AND timestamp >= $2
            ORDER BY name, pid, timestamp DESC
            ",
            &[&hostname, &window_start],
        )
        .await?;

    for row in rows {
        let name: String = row.get(0);
        let pid: i32 = row.get(1);
        let ppid: i32 = row.get(2);
        let exe: String = row.get(3);
        let cpu: f64 = row.get(4);
        let create_time: i64 = row.get(5);
        let timestamp: i64 = row.get(6);

        // --- Detector 1: path mismatch ---
        // has this name run from a DIFFERENT real path before?
        if exe != "unknown" {
            let path_mismatch = client
                .query_opt(
                    "SELECT 1 FROM processes
                     WHERE hostname = $1 AND name = $2 AND exe != 'unknown' AND exe != $3 AND timestamp < $4
                     LIMIT 1",
                    &[&hostname, &name, &exe, &timestamp],
                )
                .await?;

            if path_mismatch.is_some() {
                let alert = crate::alerts::Alert {
                    hostname: hostname.to_string(),
                    detector: "path_mismatch".to_string(),
                    severity: "high".to_string(),
                    message: format!("process '{name}' (pid {pid}) running from a new path: {exe}"),
                    category: "process".to_string(),
                };
                save_alert(client, &alert).await?;
            }
        }

        // --- Detector 2: new parent ---
        // only flag if we HAVE history for this name at all (skip brand-new processes)
        let has_history = client
            .query_opt(
                "SELECT 1 FROM processes WHERE hostname = $1 AND name = $2 AND timestamp < $3 LIMIT 1",
                &[&hostname, &name, &timestamp],
            )
            .await?;

        if has_history.is_some() {
            let ppid_seen = client
                .query_opt(
                    "SELECT 1 FROM processes WHERE hostname = $1 AND name = $2 AND ppid = $3 AND timestamp < $4 LIMIT 1",
                    &[&hostname, &name, &ppid, &timestamp],
                )
                .await?;

            if ppid_seen.is_none() {
                let alert = crate::alerts::Alert {
                    hostname: hostname.to_string(),
                    detector: "new_parent".to_string(),
                    severity: "warning".to_string(),
                    message: format!("process '{name}' (pid {pid}) has a new parent pid: {ppid}"),
                    category: "process".to_string(),
                };
                save_alert(client, &alert).await?;
            }
        }

        // --- Detector 3: young process + high CPU ---
        let age_secs = (now * 1000 - create_time) / 1000; // create_time is stored in ms
        if age_secs >= 0 && age_secs < 60 && cpu > 50.0 {
            let alert = crate::alerts::Alert {
                hostname: hostname.to_string(),
                detector: "young_high_cpu".to_string(),
                severity: "high".to_string(),
                message: format!("process '{name}' (pid {pid}) is {age_secs}s old and already at {cpu:.1}% CPU"),
                category: "process".to_string(),
            };
            save_alert(client, &alert).await?;
        }
    }

    Ok(())
}

pub async fn compute_risk_score(client: &Client, hostname: &str) -> Result<()> {
    let now = SystemTime::now().duration_since(UNIX_EPOCH)?.as_secs() as i64;
    let since = now - 900; // 15-minute window

    let rows = client
        .query(
            "SELECT severity, occurrence_count FROM alerts WHERE hostname = $1 AND last_seen >= $2",
            &[&hostname, &since],
        )
        .await?;

    let mut score = 0.0;
    for row in rows {
        let severity: String = row.get(0);
        let occurrence_count: i32 = row.get(1);

        let weight = match severity.as_str() {
            "critical" => 10.0,
            "high" => 5.0,
            "warning" => 2.0,
            _ => 1.0,
        };

        // repeated occurrences add weight, but with diminishing returns (sqrt)
        score += weight * (occurrence_count as f64).sqrt();
    }

    client
        .execute(
            "INSERT INTO host_risk (hostname, score, updated_at) VALUES ($1, $2, $3)
             ON CONFLICT (hostname) DO UPDATE SET score = $2, updated_at = $3",
            &[&hostname, &score, &now],
        )
        .await?;

    println!("[risk] {}  score={:.1}", hostname, score);
    Ok(())
}

pub async fn analyze_combination(client: &Client, hostname: &str) -> Result<()> {
    let now = SystemTime::now().duration_since(UNIX_EPOCH)?.as_secs() as i64;
    let since = now - 300; // 5-minute window

    let rows = client
        .query(
            "SELECT DISTINCT category, detector FROM alerts WHERE hostname = $1 AND last_seen >= $2",
            &[&hostname, &since],
        )
        .await?;

    if rows.len() < 2 {
        return Ok(()); // need at least two distinct signals to call it a combination
    }

    let mut has_cpu = false;
    let mut has_connections = false;
    let mut has_process_stat = false; // zscore/mad/etc firing on a process series
    let mut has_path_mismatch = false;
    let mut has_new_parent = false;
    let mut has_young_high_cpu = false;

    let mut categories_seen: Vec<String> = Vec::new();

    for row in &rows {
        let category: String = row.get(0);
        let detector: String = row.get(1);

        if !categories_seen.contains(&category) {
            categories_seen.push(category.clone());
        }

        match detector.as_str() {
            "path_mismatch" => has_path_mismatch = true,
            "new_parent" => has_new_parent = true,
            "young_high_cpu" => has_young_high_cpu = true,
            "zscore" | "mad" | "moving_average" | "ewma" => {
                if category == "cpu" {
                    has_cpu = true;
                } else if category == "connections" {
                    has_connections = true;
                } else if category == "process" {
                    has_process_stat = true;
                }
            }
            _ => {}
        }
    }

    // rules, checked in order of severity/specificity
    let label = if (has_path_mismatch || has_new_parent || has_young_high_cpu) && has_connections {
        "possible_intrusion"
    } else if has_path_mismatch || has_new_parent {
        "suspicious_process_change"
    } else if has_cpu && has_process_stat {
        "resource_exhaustion"
    } else if categories_seen.len() >= 2 {
        "uncategorized_combination"
    } else {
        return Ok(()); // shouldn't really hit this given the len check above, but just in case
    };

    let severity = if label == "possible_intrusion" { "critical" } else { "high" };

    let alert = crate::alerts::Alert {
        hostname: hostname.to_string(),
        detector: "classification".to_string(),
        severity: severity.to_string(),
        message: format!("Classified as '{label}' — categories seen: {}", categories_seen.join(", ")),
        category: label.to_string(),
    };
    save_alert(client, &alert).await?;

    Ok(())
}