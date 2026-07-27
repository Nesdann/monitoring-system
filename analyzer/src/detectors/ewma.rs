use super::Detector;
use crate::alerts::Alert;
use crate::statistics::ewma;
use crate::severity::severity_from_ratio;

pub struct EwmaDetector{
    pub deviation_threshold: f64,
    pub alpha: f64,
}
impl Detector for EwmaDetector{
    fn analyze(&self, hostname: &str, samples: &[f64]) -> Option<Alert> {
        let current = *samples.first()?;
        let baseline = ewma(samples, self.alpha)?;

        if baseline == 0.0 {
            return None;
        }

        let deviation = (current - baseline).abs() / baseline;

        if deviation > self.deviation_threshold {
            let ratio = deviation / self.deviation_threshold;
            println!("Alert for host: {}, current={:.2}, ewma={:.2}, deviation={:.0}%", hostname, current, baseline, deviation * 100.0);
            Some(Alert {
                hostname: hostname.to_string(),
                detector: "ewma".to_string(),
                severity: severity_from_ratio(ratio),

                message: format!(
                    "current={:.2}, ewma={:.2}, deviation={:.0}%",
                    current, baseline, deviation * 100.0
                ),
                category: String::new(),
            })
        } else {
            None
        }
    }
}