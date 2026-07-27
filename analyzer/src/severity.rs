pub fn severity_from_ratio(ratio: f64) -> String {
    let ratio = ratio.abs();
    if ratio >= 2.0 {
        "critical".to_string()
    } else if ratio >= 1.5 {
        "high".to_string()
    } else {
        "warning".to_string()
    }
}