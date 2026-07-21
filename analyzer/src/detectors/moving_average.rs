use super::Detector;
use crate::alerts::Alert;
use crate::statistics::moving_average;

pub struct MovingAverageDetector{
    pub deviation_threshold: f64,
    pub window_size: usize,
}

impl Detector for MovingAverageDetector{
    fn analyze(&self, hostname:&str, samples:&[f64])-> Option<Alert>{
        let current = *samples.first()?;
        let window: Vec<f64> = samples.iter().take(self.window_size).cloned().collect();

        let avg = moving_average(&window)?;
        if avg == 0.0 {
            return None;
        }

        let deviation = (current - avg).abs() / avg;
        println!("No alert for host: {}, current={:.2}, moving_avg={:.2}, deviation={:.0}%", hostname, current, avg, deviation * 100.0);
        if deviation > self.deviation_threshold {
            Some(Alert {
                hostname: hostname.to_string(),
                detector: "moving_average".to_string(),
                severity: "warning".to_string(),
                message: format!(
                    "current={:.2}, moving_avg={:.2}, deviation={:.0}%",
                    current, avg, deviation * 100.0
                ),
                category: String::new(),
            })
        } else {
            None
        }
    }
}