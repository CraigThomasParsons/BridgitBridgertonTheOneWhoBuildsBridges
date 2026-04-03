pub mod api;
pub mod extractor;
pub mod models;
pub mod packager;

use anyhow::{Context, Result};
use std::env;

#[tokio::main]
async fn main() -> Result<()> {
    // 1. Ensure minimal env overrides and load configuration safely 
    //    so the pipeline predictably fails straight away if incorrectly provisioned.
    dotenvy::dotenv().ok();

    // 2. We operate on a project ID argument, simulating the daemon or cron invocation.
    let args: Vec<String> = env::args().collect();
    if args.len() < 2 {
        eprintln!("Usage: chatprojectstokraxbridge <project_id>");
        std::process::exit(1);
    }

    let project_id_str = &args[1];
    let project_id: u64 = project_id_str
        .parse()
        .context("Project ID must be a valid positive integer")?;

    println!("Starting extraction pipeline for ChatProjects project {}", project_id);

    // 3. Fetch the strongly-typed Krax Context payload directly from the source API.
    println!("Fetching context from ChatProjects API...");
    let response = api::fetch_krax_input(project_id).await?;

    // 4. Transform the structured Inception data into the deterministic "Hard Context"
    //    This explicit textual format cannot be hallucinated.
    let hard_context = if let Some(ref inc) = response.inception {
        format!(
            "VISION: {}\nGOALS: {}\n\nFEATURES TO IMPLEMENT:\n{}",
            inc.vision_statement.as_deref().unwrap_or("None"),
            inc.business_goals.as_deref().unwrap_or("None"),
            inc.features.iter().map(|f| format!("- {} (Value: {}, Effort: {})", f.name, f.value.as_deref().unwrap_or("N/A"), f.effort.as_deref().unwrap_or("N/A"))).collect::<Vec<String>>().join("\n")
        )
    } else {
        "No Inception Data provided. Do not expand features.".to_string()
    };

    // 5. Transform the raw transcripts into "Soft Context".
    let soft_context = response.conversations.iter()
        .filter_map(|c| c.raw_content.clone())
        .collect::<Vec<String>>()
        .join("\n\n---\n\n");

    // 6. Push into LLM extraction execution phase (OpenAI)
    //    We only expand what is mandated by the pipeline hard context.
    println!("Sending context payload to OpenAI for expansion extraction...");
    let extracted_payload = extractor::expand_artifacts(&hard_context, &soft_context).await?;

    // 7. Strictly slice and parse the `===FILE===` returns.
    let parsed_artifacts = extractor::parse_raw_llm_output(&extracted_payload)?;
    
    // 8. Output strictly to the physical Outbox contract boundary
    //    The PostalService will safely scoop up the result.
    println!("Writing fully materialized deterministic files into Outbox...");
    packager::execute_packaging_contract(
        project_id,
        &response,
        parsed_artifacts.epics,
        parsed_artifacts.stories,
        parsed_artifacts.constraints,
    )?;

    println!("Handoff pipeline completed successfully for Project {}.", project_id);
    Ok(())
}
