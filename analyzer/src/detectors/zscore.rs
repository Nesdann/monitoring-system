use super::Detector;
use crate::alerts::Alert;
use crate::statistics::z_score;

pub struct ZScoreDetector {
    pub threshold: f64,
}

impl Detector for ZScoreDetector {
    fn analyze(&self, hostname: &str, samples: &[f64]) -> Option<Alert> {
        let current = *samples.first()?;
        let z = z_score(current, samples)?;

        if z.abs() > self.threshold {
            Some(Alert {
                hostname: hostname.to_string(),
                detector: "zscore".to_string(),
                severity: "critical".to_string(),
                message: format!("z={:.2}, current={:.2}", z, current),
                category: String::new(),
            })
        } else {
            println!("No alert for host: {}, z={:.2}, current={:.2}", hostname, z, current);
            None
        }
    }
}