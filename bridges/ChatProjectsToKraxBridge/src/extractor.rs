use anyhow::{Context, Result};
use reqwest::Client;
use serde_json::{json, Value};
use std::fs;

/// Calls the OpenAI API (or compatible completion endpoint)
/// using the supplied hard context and soft context.
pub async fn expand_artifacts(
    hard_context: &str,
    soft_context: &str,
) -> Result<String> {
    let api_key = std::env::var("OPENAI_API_KEY")
        .context("OPENAI_API_KEY must be set in env for artifact extraction")?;

    let system_prompt =
        fs::read_to_string("prompts/extract_artifacts.md").context("Failed to load prompt")?;

    let user_message = format!(
        "=== [HARD CONTEXT] ===\n\n{}\n\n=== [SOFT CONTEXT] ===\n\n{}\n\n=== END OF CONTEXT ===",
        hard_context, soft_context
    );

    let client = Client::new();

    // Call out to the chat completion API
    // We use gpt-4o as it excels at following exact formatting instructions
    let response = client
        .post("https://api.openai.com/v1/chat/completions")
        .bearer_auth(api_key)
        .json(&json!({
            "model": "gpt-4o",
            "messages": [
                {
                    "role": "system",
                    "content": system_prompt
                },
                {
                    "role": "user",
                    "content": user_message
                }
            ],
            "temperature": 0.2 // Keep deterministic
        }))
        .send()
        .await
        .context("Failed executing OpenAI request")?;

    if !response.status().is_success() {
        return Err(anyhow::anyhow!(
            "OpenAI API failed with status: {}, body: {:?}",
            response.status(),
            response.text().await
        ));
    }

    let payload: Value = response
        .json()
        .await
        .context("Failed to parse OpenAI JSON response")?;

    // Extract the exact text message string produced by the LLM
    let extracted_content = payload["choices"][0]["message"]["content"]
        .as_str()
        .context("Failed to extract completion content from OpenAI metadata")?
        .to_string();

    Ok(extracted_content)
}

/// A parsed collection of generated Markdown files.
#[derive(Debug)]
pub struct ParsedArtifacts {
    pub epics: Option<String>,
    pub stories: Option<String>,
    pub constraints: Option<String>,
}

/// Slices the raw LLM output explicitly marked by `===FILE: [NAME]===`
/// into individual string blocks.
pub fn parse_raw_llm_output(output: &str) -> Result<ParsedArtifacts> {
    let mut epics = None;
    let mut stories = None;
    let mut constraints = None;

    let parts: Vec<&str> = output.split("===FILE: ").collect();

    // Loop through the split regions and match exact file targets
    for part in parts {
        let block_source = part.trim();
        if block_source.is_empty() {
            continue;
        }

        // Search the explicit divider marker 
        if let Some((filename, content)) = block_source.split_once("===") {
            let filename = filename.trim();
            let content = content.trim().to_string();

            // Store safely into strongly typed container
            match filename {
                "EPICS.md" => epics = Some(content),
                "STORIES.md" => stories = Some(content),
                "CONSTRAINTS.md" => constraints = Some(content),
                _ => {
                    // Ignore hallucinated files rather than blindly accepting them
                    eprintln!("Warning: LLM hallucinated unknown file '{}'", filename);
                }
            }
        }
    }

    Ok(ParsedArtifacts {
        epics,
        stories,
        constraints,
    })
}
