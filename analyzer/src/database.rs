use tokio_postgres::Client;
use anyhow::Result;
use crate::detectors::zscore::ZScoreDetector;
use crate::detectors::moving_average::MovingAverageDetector;
use crate::detectors::Detector;
use crate::alerts::{Alert, save_alert};
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
            "
            SELECT cpu
            FROM metrics
            WHERE hostname = $1
            ORDER BY timestamp DESC
            LIMIT 30
            ",
            &[&hostname],
        )
        .await?;

    Ok(rows.into_iter().map(|row| row.get(0)).collect())
}

pub async fn analyze_host(client: &Client, hostname: &str) -> Result<()> {
    println!("Analyzing host take samples: {}", hostname);
    let samples = get_cpu_samples(client, hostname).await?;
    
    if samples.is_empty() {
        println!("No samples for host: {}", hostname);
        return Ok(());
    }
    
    let detector = ZScoreDetector { threshold: 2.5 };
    if let Some(alert) = detector.analyze(hostname, &samples) {
        save_alert(client, &alert).await?;
    }
    
    let moving_avg_detector = MovingAverageDetector {
        deviation_threshold: 0.3,
        window_size: 10,
    };
    if let Some(alert) = moving_avg_detector.analyze(hostname, &samples) {
        save_alert(client, &alert).await?;
    }

    let ewma_detector = EwmaDetector {
        deviation_threshold: 0.3,
        alpha: 0.5,
    };
    if let Some(alert) = ewma_detector.analyze(hostname, &samples) {
        save_alert(client, &alert).await?;
    }

    let mad_detector = MadDetector {
        threshold: 2.5,
    };
    if let Some(alert) = mad_detector.analyze(hostname, &samples) {
        save_alert(client, &alert).await?;
    }

    Ok(())
}