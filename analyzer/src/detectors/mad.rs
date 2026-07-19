use super::Detector;
use crate::alerts::Alert;
use crate::statistics::mad_score;

pub struct MadDetector {
    pub threshold: f64,
}

impl Detector for MadDetector {
    fn analyze(&self, hostname: &str, samples: &[f64]) -> Option<Alert> {
        let current = *samples.first()?;
        let score = mad_score(current, samples)?;

        if score.abs() > self.threshold {
            Some(Alert {
                hostname: hostname.to_string(),
                detector: "mad".to_string(),
                severity: "warning".to_string(),
                message: format!("current={:.2}, mad_score={:.2}", current, score),
            })
        } else {
            None
        }
    }
}