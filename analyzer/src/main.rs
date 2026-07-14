use anyhow::Result;
use tokio_postgres::NoTls;

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

    println!("Connected to PostgreSQL!");

    let rows = client
        .query(
            "
            SELECT cpu
            FROM metrics
            WHERE hostname = $1
            ORDER BY timestamp DESC
            LIMIT 30
            ",
            &[&"agent-1"],
        )
        .await?;

    let mut samples = Vec::new();

    for row in rows {
        let cpu: f64 = row.get(0);
        samples.push(cpu);
    }

    if samples.len() < 2 {
        println!("Not enough samples.");
        return Ok(());
    }

    let avg = mean(&samples);
    let sigma = std_dev(&samples, avg);
    let current = samples[0];

    let z = (current - avg) / sigma;

    println!("Current CPU: {:.2}", current);
    println!("Mean: {:.2}", avg);
    println!("Std Dev: {:.2}", sigma);
    println!("Z-Score: {:.2}", z);

    if z.abs() > 3.0 {
        println!("ALERT! CPU anomaly detected.");
    } else {
        println!("CPU is normal.");
    }

    Ok(())
}