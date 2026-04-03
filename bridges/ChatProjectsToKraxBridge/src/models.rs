use serde::Deserialize;

#[derive(Debug, Deserialize)]
pub struct KraxInputResponse {
    pub project: ProjectData,
    pub inception: Option<InceptionData>,
    pub conversations: Vec<ConversationData>,
}

#[derive(Debug, Deserialize)]
pub struct ProjectData {
    pub id: u64,
    pub name: String,
    pub description: Option<String>,
    pub code_context: CodeContextData,
}

#[derive(Debug, Deserialize)]
pub struct CodeContextData {
    pub local_location: Option<String>,
    pub github_repo: Option<String>,
    pub framework_description: Option<String>,
    pub languages: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct InceptionData {
    pub id: u64,
    pub business_goals: Option<String>,
    pub success_metrics: Option<Vec<String>>,
    pub vision_statement: Option<String>,
    pub mvp_canvas: Option<serde_json::Value>,
    pub personas: Vec<PersonaData>,
    pub features: Vec<FeatureData>,
}

#[derive(Debug, Deserialize)]
pub struct PersonaData {
    pub name: String,
    pub profile: Option<String>,
    pub needs: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct FeatureData {
    pub name: String,
    pub description: Option<String>,
    pub value: Option<String>,
    pub effort: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct ConversationData {
    pub id: u64,
    pub title: Option<String>,
    pub raw_content: Option<String>,
    pub updated_at: Option<String>,
}
