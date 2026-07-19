use crate::alerts::Alert;

pub trait Detector {
    fn analyze(&self, hostname: &str, samples: &[f64]) -> Option<Alert>;
}

pub mod zscore;
pub mod moving_average;
pub mod ewma;
pub mod mad;