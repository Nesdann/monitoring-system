use anyhow::Result;
use std::time::{SystemTime, UNIX_EPOCH};
use tokio::time::{sleep, Duration};
use tokio_postgres::{Client, NoTls};

fn mean(values: &[f64]) -> f64 {
    values.iter().sum::<f64>() / values.len() as f64
}

fn std_dev(values: &[f64], mean: f64) -> f64 {
    let variance = values
        .iter()
        .map(|x| (x - mean).powi(2))
        .sum::<f64>()
        / values.len() as f64;
    variance.sqrt()
}

async fn get_hosts(client: &Client) -> Result<Vec<String>> {
    let rows = client
        .query("SELECT DISTINCT hostname FROM metrics", &[])
        .await?;

    Ok(rows.into_iter().map(|row| row.get(0)).collect())
}

async fn get_cpu_samples(client: &Client, hostname: &str) -> Result<Vec<f64>> {
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

async fn save_alert(client: &Client, hostname: &str, message: &str) -> Result<()> {
    let ts = SystemTime::now()
        .duration_since(UNIX_EPOCH)?
        .as_secs() as i64;

    client
        .execute(
            "
            INSERT INTO alerts (timestamp, hostname, detector, severity, message)
            VALUES ($1, $2, $3, $4, $5)
            ",
            &[&ts, &hostname, &"zscore", &"critical", &message],
        )
        .await?;

    Ok(())
}

async fn analyze_host(client: &Client, hostname: &str) -> Result<()> {
    let samples = get_cpu_samples(client, hostname).await?;

    if samples.len() < 2 {
        println!("[{}] Not enough samples, skipping.", hostname);
        return Ok(());
    }

    let avg = mean(&samples);
    let sigma = std_dev(&samples, avg);

    if sigma == 0.0 {
        println!("[{}] No variance, skipping.", hostname);
        return Ok(());
    }

    let current = samples[0];
    let z = (current - avg) / sigma;

    println!(
        "[{}] current={:.2} mean={:.2} std={:.2} z={:.2}",
        hostname, current, avg, sigma, z
    );

    if z.abs() > 3.0 {
        let message = format!(
            "CPU anomaly on {}: current={:.2}, mean={:.2}, z={:.2}",
            hostname, current, avg, z
        );
        println!("ALERT! {}", message);
        save_alert(client, hostname, &message).await?;
    }

    Ok(())
}

#[tokio::main]
async fn main() -> Result<()> {
    let (client, connection) = tokio_postgres::connect(
        "host=localhost user=monitoring password=monitoring123 dbname=monitoring sslmode=disable",
        NoTls,
    )
    .await?;

    tokio::spawn(async move {
        if let Err(e) = connection.await {
            eprintln!("Database error: {}", e);
        }
    });

    println!("Analyzer started. Checking every 30 seconds...");

    loop {
        match get_hosts(&client).await {
            Ok(hosts) => {
                for hostname in hosts {
                    if let Err(e) = analyze_host(&client, &hostname).await {
                        eprintln!("Error analyzing {}: {}", hostname, e);
                    }
                }
            }
            Err(e) => eprintln!("Error fetching hosts: {}", e),
        }

        sleep(Duration::from_secs(30)).await;
    }
}