pub struct Alert {
    pub hostname: String,
    pub detector: String,
    pub severity: String,
    pub message: String,
}

use tokio_postgres::Client;
use anyhow::Result;
use std::time::{SystemTime, UNIX_EPOCH};

pub async fn save_alert(client: &Client, alert: &Alert) -> Result<()> {
    let ts = SystemTime::now().duration_since(UNIX_EPOCH)?.as_secs() as i64;
    client
        .execute(
            "INSERT INTO alerts (timestamp, hostname, detector, severity, message) VALUES ($1, $2, $3, $4, $5)",
            &[&ts, &alert.hostname, &alert.detector, &alert.severity, &alert.message],
        )
        .await?;
    Ok(())
}