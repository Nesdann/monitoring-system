mod database;
mod statistics;
mod alerts;
mod detectors;

use anyhow::Result;
use tokio::time::{sleep, Duration};
use tokio_postgres::NoTls;
use database::{get_hosts, analyze_host, analyze_processes};


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
                         eprintln!("Error analyzing {}: {:?}", hostname, e);
                    }
                    if let Err(e) = analyze_processes(&client, &hostname).await {
                          eprintln!("Error analyzing processes {}: {:?}", hostname, e);
                      }
                }
                   
            }
            Err(e) => eprintln!("Error fetching hosts: {}", e),
        }

        sleep(Duration::from_secs(30)).await;
    }
}