pub fn mean(values: &[f64]) -> f64 {
    values.iter().sum::<f64>() / values.len() as f64
}

pub fn std_dev(values: &[f64], mean: f64) -> f64 {
    let variance = values.iter().map(|x| (x - mean).powi(2)).sum::<f64>() / values.len() as f64;
    variance.sqrt()
}

pub fn z_score(current: f64, values: &[f64]) -> Option<f64> {
    if values.len() < 2 {
        return None;
    }
    let avg = mean(values);
    let sigma = std_dev(values, avg);
    if sigma == 0.0 {
        return None;
    }
    Some((current - avg) / sigma)
}

pub fn moving_average(values: &[f64]) -> Option<f64> {
    if values.is_empty() {
        return None;
    }
    Some(values.iter().sum::<f64>() / values.len() as f64)
}

pub fn ewma(values: &[f64], alpha: f64) -> Option<f64> {
    let mut iter = values.iter().rev(); // oldest first
    let first = *iter.next()?;
    Some(iter.fold(first, |acc, &x| alpha * x + (1.0 - alpha) * acc))
}

pub fn median(values: &[f64]) -> Option<f64> {
    if values.is_empty() {
        return None;
    }
    let mut sorted = values.to_vec();
    sorted.sort_by(|a, b| a.partial_cmp(b).unwrap());
    let mid = sorted.len() / 2;
    if sorted.len() % 2 == 0 {
        Some((sorted[mid - 1] + sorted[mid]) / 2.0)
    } else {
        Some(sorted[mid])
    }
}

pub fn mad_score(current: f64, values: &[f64]) -> Option<f64> {
    let med = median(values)?;
    let deviations: Vec<f64> = values.iter().map(|x| (x - med).abs()).collect();
    let mad = median(&deviations)?;

    if mad == 0.0 {
        return None;
    }

    Some(0.6745 * (current - med) / mad)
}