use crate::models::KraxInputResponse;
use anyhow::{Context, Result};
use reqwest::Client;

/// Fetch the heavily structured `krax-input` payload from ChatProjects.
///
/// This requires the environment variables `CHAT_PROJECTS_URL` and `KRAX_TOKEN`
/// to be set and accessible via the system environment or a `.env` file.
pub async fn fetch_krax_input(project_id: u64) -> Result<KraxInputResponse> {
    // Read the base URL and API token from the environment configuration.
    // If these are missing, we explicitly fail early before making network calls.
    let base_url =
        std::env::var("CHAT_PROJECTS_URL").context("CHAT_PROJECTS_URL must be set in env")?;
    let token = std::env::var("KRAX_TOKEN").context("KRAX_TOKEN must be set in env")?;

    // Construct the explicit route to the Krax Input API.
    // Ensure no double slashes if the base URL has a trailing slash.
    let trimmed_base_url = base_url.trim_end_matches('/');
    let url = format!("{}/api/projects/{}/krax-input", trimmed_base_url, project_id);

    // Initialize the HTTP client. 
    // We instantiate a new client per fetch for simplicity; 
    // in a high-throughput daemon setting this should be lifted and re-used.
    let client = Client::new();

    // Fire the GET request with the required Bearer token auth.
    let response = client
        .get(&url)
        .bearer_auth(token)
        .send()
        .await
        .context(format!("Failed to execute HTTP request to {}", url))?;

    // Explicitly check for non-200 HTTP status codes,
    // meaning the project might not exist or auth failed.
    if !response.status().is_success() {
        return Err(anyhow::anyhow!(
            "API request failed with status: {}",
            response.status()
        ));
    }

    // Attempt to deserialize the JSON payload into our strict Rust domain model.
    let payload_body: KraxInputResponse = response
        .json()
        .await
        .context("Failed to deserialize the KraxInputResponse JSON payload")?;

    Ok(payload_body)
}
