use rand::Rng;
use super::Detector;
use crate::alerts::Alert;

enum Node {
    Leaf { size: usize },
    Split {
        split_value: f64,
        left: Box<Node>,
        right: Box<Node>,
    },
}

fn build_tree(data: &[f64], depth: usize, max_depth: usize) -> Node {
    if data.len() <= 1 || depth >= max_depth {
        return Node::Leaf { size: data.len() };
    }

    let min = data.iter().cloned().fold(f64::INFINITY, f64::min);
    let max = data.iter().cloned().fold(f64::NEG_INFINITY, f64::max);

    if min == max {
        return Node::Leaf { size: data.len() };
    }

    let mut rng = rand::thread_rng();
    let split_value = rng.gen_range(min..max);

    let left: Vec<f64> = data.iter().cloned().filter(|&x| x < split_value).collect();
    let right: Vec<f64> = data.iter().cloned().filter(|&x| x >= split_value).collect();

    Node::Split {
        split_value,
        left: Box::new(build_tree(&left, depth + 1, max_depth)),
        right: Box::new(build_tree(&right, depth + 1, max_depth)),
    }
}

fn path_length(node: &Node, value: f64, depth: usize) -> f64 {
    match node {
        Node::Leaf { size } => depth as f64 + c_factor(*size),
        Node::Split { split_value, left, right } => {
            if value < *split_value {
                path_length(left, value, depth + 1)
            } else {
                path_length(right, value, depth + 1)
            }
        }
    }
}

// average path length of an unsuccessful search in a binary tree — the normalizing constant
fn c_factor(n: usize) -> f64 {
    if n <= 1 {
        return 0.0;
    }
    let n = n as f64;
    2.0 * (n.ln() + 0.5772156649) - (2.0 * (n - 1.0) / n)
}

pub struct IsolationForestDetector {
    pub num_trees: usize,
    pub threshold: f64, // typically ~0.6
}

impl Detector for IsolationForestDetector {
    fn analyze(&self, hostname: &str, samples: &[f64]) -> Option<Alert> {
        if samples.len() < 10 {
            return None; // not enough data to build meaningful trees
        }

        let current = *samples.first()?;
        let max_depth = (samples.len() as f64).log2().ceil() as usize;

        let trees: Vec<Node> = (0..self.num_trees)
            .map(|_| build_tree(samples, 0, max_depth))
            .collect();

        let avg_path: f64 = trees.iter()
            .map(|t| path_length(t, current, 0))
            .sum::<f64>() / self.num_trees as f64;

        let score = 2f64.powf(-avg_path / c_factor(samples.len()));

        if score > self.threshold {
            Some(Alert {
                hostname: hostname.to_string(),
                detector: "isolation_forest".to_string(),
                severity: "warning".to_string(),
                message: format!("current={:.2}, anomaly_score={:.2}", current, score),
            })
        } else {
            None
        }
    }
}