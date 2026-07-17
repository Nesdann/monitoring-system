use crate::alerts::Alert;

pub trait Detector {
    fn analyze(&self, hostname: &str, samples: &[f64]) -> Option<Alert>;
}

pub mod zscore;