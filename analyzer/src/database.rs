use tokio_postgres::Client;
use anyhow::Result;
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

// NEW: analyze_processes — one cpu+mem series PER process name, not per host
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