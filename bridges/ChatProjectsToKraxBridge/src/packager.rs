use crate::models::KraxInputResponse;
use anyhow::{Context, Result};
use std::fs;

/// Write the successfully assembled static artifacts into the outbox
/// for the PostalService to move exactly into Krax.
///
/// Converts the heavy API model + the LLM strings into pure Markdown.
pub fn execute_packaging_contract(
    project_id: u64,
    response: &KraxInputResponse,
    epics: Option<String>,
    stories: Option<String>,
    constraints: Option<String>,
) -> Result<()> {
    // Scaffold out the outbox payload folder based on universally agreed project id.
    let package_path = format!("outbox/project-{}", project_id);
    fs::create_dir_all(&package_path)
        .context(format!("Failed to create package directory at {}", package_path))?;

    // Hard-Generate VISION.md purely from the Inception DB structured fields.
    // If there is no inception data, write an empty static stub so downstream pipelines
    // don't physically crash trying to read missing boundaries.
    let vision_content = if let Some(ref inc) = response.inception {
        format!(
            "# Vision Statement\n{}\n\n# Business Goals\n{}\n\n# Metrics\n{:?}\n",
            inc.vision_statement.as_deref().unwrap_or("No specific vision defined."),
            inc.business_goals.as_deref().unwrap_or("No business goals defined."),
            inc.success_metrics.as_ref().map(|m| format!("- {}", m.join("\n- "))).unwrap_or_else(|| "No specific metrics.".to_string())
        )
    } else {
        "# Vision Statement\nNo inception data available for this project.\n".to_string()
    };
    
    fs::write(format!("{}/VISION.md", package_path), vision_content)
        .context("Failed to write VISION.md")?;

    // Hard-Generate PERSONAS.md purely from Lean Inception DB relations.
    let mut personas_content = String::from("# Personas\n\n");
    if let Some(ref inc) = response.inception {
        if inc.personas.is_empty() {
            personas_content.push_str("No distinct personas defined in Lean Inception.\n");
        } else {
            for p in &inc.personas {
                personas_content.push_str(&format!(
                    "## {}\n\n**Profile:** {}\n\n**Needs:** {}\n\n",
                    p.name,
                    p.profile.as_deref().unwrap_or("N/A"),
                    p.needs.as_deref().unwrap_or("N/A")
                ));
            }
        }
    }
    
    fs::write(format!("{}/PERSONAS.md", package_path), personas_content)
        .context("Failed to write PERSONAS.md")?;

    // Write out the LLM generated string segments if they cleanly parsed.
    if let Some(e) = epics {
        fs::write(format!("{}/EPICS.md", package_path), e)
            .context("Failed to write EPICS.md")?;
    }
    if let Some(s) = stories {
        fs::write(format!("{}/STORIES.md", package_path), s)
            .context("Failed to write STORIES.md")?;
    }
    if let Some(c) = constraints {
        fs::write(format!("{}/CONSTRAINTS.md", package_path), c)
            .context("Failed to write CONSTRAINTS.md")?;
    }

    // Write out the exact tracking contract mapping for ThePostalService
    let letter_toml = format!(
        "recipient = \"krax\"\nproject_id = {}\nstage = \"extracted\"\n",
        project_id
    );

    fs::write(format!("{}/letter.toml", package_path), letter_toml)
        .context("Failed to write letter.toml routing contract")?;

    Ok(())
}
