use anyhow::{Context, Result, bail};
use axum::{
    Json, Router,
    body::{Body, Bytes},
    extract::{Query, State},
    http::{HeaderMap, Method, Request, Response, StatusCode},
    response::IntoResponse,
    routing::{get, post},
};
use base64::{Engine as _, engine::general_purpose};
use chrono::{DateTime, Utc};
use hyper::{body::Incoming, server::conn::http1, service::service_fn, upgrade};
use hyper_util::rt::TokioIo;
use reqwest::Client;
use serde::{Deserialize, Serialize};
use std::{
    convert::Infallible,
    env,
    net::SocketAddr,
    path::{Path, PathBuf},
    sync::Arc,
    time::{Duration, SystemTime},
};
use tokio::{
    io::copy_bidirectional,
    net::{TcpListener, TcpStream},
    signal,
};
use tower::ServiceExt;
use tracing::{error, info};
use tracing_subscriber::{EnvFilter, fmt};
use walkdir::WalkDir;

const CAP_UPLOAD: &str = "upload";
const CAP_FILESCAN: &str = "filescan";

#[derive(Clone, Debug)]
struct AppState {
    token: String,
    purpose: AgentPurpose,
    listen: SocketAddr,
    work_path: PathBuf,
    upstream_base_url: String,
    upstream_token: String,
    http_client: Client,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "lowercase")]
enum AgentPurpose {
    Upload,
    Filescan,
    Both,
}

impl AgentPurpose {
    fn parse(raw: &str) -> Self {
        match raw.trim().to_ascii_lowercase().as_str() {
            "upload" | "publish" => Self::Upload,
            "filescan" | "filecheck" | "check" => Self::Filescan,
            _ => Self::Both,
        }
    }

    fn as_str(self) -> &'static str {
        match self {
            Self::Upload => "upload",
            Self::Filescan => "filescan",
            Self::Both => "both",
        }
    }

    fn allows(self, cap: &str) -> bool {
        matches!(
            (self, cap),
            (Self::Both, _) | (Self::Upload, CAP_UPLOAD) | (Self::Filescan, CAP_FILESCAN)
        )
    }

    fn capabilities(self) -> Vec<&'static str> {
        match self {
            Self::Upload => vec![CAP_UPLOAD],
            Self::Filescan => vec![CAP_FILESCAN],
            Self::Both => vec![CAP_UPLOAD, CAP_FILESCAN],
        }
    }
}

#[derive(Debug, Deserialize, Serialize)]
struct PublishRequest {
    #[serde(rename = "historyId")]
    history_id: u64,
    #[serde(rename = "userId")]
    user_id: u64,
}

#[derive(Debug, Deserialize)]
struct FileCheckParams {
    limit: Option<usize>,
}

#[derive(Debug, Deserialize)]
struct FileCheckRequest {
    paths: Option<Vec<String>>,
    limit: Option<usize>,
    #[serde(rename = "minSize")]
    min_size: Option<u64>,
    extensions: Option<Vec<String>>,
}

#[derive(Debug, Serialize)]
struct ApiResponse<T: Serialize> {
    #[serde(rename = "type")]
    response_type: &'static str,
    msg: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    data: Option<T>,
}

#[derive(Debug, Serialize)]
struct HealthData {
    version: &'static str,
    purpose: String,
    capabilities: Vec<&'static str>,
    listen: String,
    #[serde(rename = "workPath")]
    work_path: String,
    #[serde(rename = "upstreamBaseUrl")]
    upstream_base_url: String,
    time: DateTime<Utc>,
}

#[derive(Debug, Serialize)]
struct FileCheckItem {
    #[serde(rename = "filePath")]
    file_path: String,
    #[serde(rename = "fileName")]
    file_name: String,
    #[serde(rename = "fileSize")]
    file_size: u64,
    #[serde(rename = "modTime")]
    mod_time: DateTime<Utc>,
}

#[derive(Debug, Serialize)]
struct FileCheckResult {
    purpose: String,
    #[serde(rename = "workPath")]
    work_path: String,
    #[serde(rename = "totalFiles")]
    total_files: usize,
    #[serde(rename = "totalSize")]
    total_size: u64,
    #[serde(rename = "sampleLimit")]
    sample_limit: usize,
    files: Vec<FileCheckItem>,
    errors: Vec<String>,
}

#[tokio::main]
async fn main() -> Result<()> {
    fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info")),
        )
        .init();

    let args: Vec<String> = env::args().collect();
    let listen = arg_or_env(&args, "--listen", "GOBUP_AGENT_LISTEN")
        .unwrap_or_else(|| "0.0.0.0:12381".to_string())
        .parse::<SocketAddr>()
        .context("invalid --listen address")?;
    let token = arg_or_env(&args, "--token", "GOBUP_AGENT_TOKEN").unwrap_or_default();
    let purpose = AgentPurpose::parse(
        &arg_or_env(&args, "--purpose", "GOBUP_AGENT_PURPOSE")
            .unwrap_or_else(|| "both".to_string()),
    );
    let work_path = arg_or_env(&args, "--work-path", "GOBUP_AGENT_WORK_PATH")
        .or_else(|| env::var("WORK_PATH").ok())
        .unwrap_or_else(|| "/rec".to_string());
    let upstream_base_url = arg_or_env(
        &args,
        "--upstream-base-url",
        "GOBUP_AGENT_UPSTREAM_BASE_URL",
    )
    .unwrap_or_else(|| "http://127.0.0.1:12380".to_string());
    let upstream_token = arg_or_env(&args, "--upstream-token", "GOBUP_AGENT_UPSTREAM_TOKEN")
        .unwrap_or_else(|| token.clone());
    let timeout_secs = arg_or_env(&args, "--timeout", "GOBUP_AGENT_TIMEOUT")
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(30)
        .clamp(3, 600);

    let state = Arc::new(AppState {
        token,
        purpose,
        listen,
        work_path: PathBuf::from(work_path),
        upstream_base_url: upstream_base_url.trim_end_matches('/').to_string(),
        upstream_token,
        http_client: Client::builder()
            .timeout(Duration::from_secs(timeout_secs))
            .build()
            .context("failed to build HTTP client")?,
    });

    let app = Router::new()
        .route("/agent/v1/health", get(health))
        .route("/agent/v1/publish", post(publish))
        .route(
            "/agent/v1/files/check",
            get(check_files_get).post(check_files_post),
        )
        .with_state(state.clone());

    let listener = TcpListener::bind(state.listen)
        .await
        .context("failed to bind listener")?;
    info!(
        listen = %state.listen,
        purpose = state.purpose.as_str(),
        work_path = %state.work_path.display(),
        "gobup agent started"
    );

    serve_agent(listener, state, app)
        .await
        .context("agent server failed")?;
    Ok(())
}

async fn serve_agent(listener: TcpListener, state: Arc<AppState>, app: Router) -> Result<()> {
    let shutdown = shutdown_signal();
    tokio::pin!(shutdown);

    loop {
        tokio::select! {
            _ = &mut shutdown => {
                info!("agent shutdown signal received");
                break;
            }
            accepted = listener.accept() => {
                let (stream, peer) = match accepted {
                    Ok(accepted) => accepted,
                    Err(err) => {
                        error!(%err, "agent accept failed");
                        continue;
                    }
                };
                let state = state.clone();
                let app = app.clone();
                tokio::spawn(async move {
                    let service = service_fn(move |req: Request<Incoming>| {
                        let state = state.clone();
                        let app = app.clone();
                        async move {
                            let response = if req.method() == Method::CONNECT {
                                proxy_connect(state, req).await
                            } else {
                                match app.oneshot(req.map(Body::new)).await {
                                    Ok(response) => response,
                                    Err(err) => {
                                        error!(%err, "agent router request failed");
                                        text_response(StatusCode::INTERNAL_SERVER_ERROR, "agent router request failed")
                                    }
                                }
                            };
                            Ok::<_, Infallible>(response)
                        }
                    });
                    let io = TokioIo::new(stream);
                    if let Err(err) = http1::Builder::new()
                        .serve_connection(io, service)
                        .with_upgrades()
                        .await
                    {
                        error!(peer = %peer, %err, "agent connection failed");
                    }
                });
            }
        }
    }
    Ok(())
}

async fn shutdown_signal() {
    let ctrl_c = async {
        if let Err(err) = signal::ctrl_c().await {
            error!(%err, "failed to install ctrl-c handler");
        }
    };

    #[cfg(unix)]
    let terminate = async {
        if let Ok(mut sigterm) = signal::unix::signal(signal::unix::SignalKind::terminate()) {
            sigterm.recv().await;
        }
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
}

async fn health(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
) -> (StatusCode, Json<ApiResponse<HealthData>>) {
    if let Err(resp) = authorize(&state, &headers) {
        return resp;
    }

    ok(
        "agent ok",
        HealthData {
            version: env!("CARGO_PKG_VERSION"),
            purpose: state.purpose.as_str().to_string(),
            capabilities: state.purpose.capabilities(),
            listen: state.listen.to_string(),
            work_path: state.work_path.display().to_string(),
            upstream_base_url: state.upstream_base_url.clone(),
            time: Utc::now(),
        },
    )
}

async fn publish(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
    body: Bytes,
) -> (StatusCode, Json<ApiResponse<serde_json::Value>>) {
    if let Err(resp) = authorize(&state, &headers) {
        return resp;
    }
    if !state.purpose.allows(CAP_UPLOAD) {
        return err(StatusCode::FORBIDDEN, "agent purpose does not allow upload");
    }
    let payload = match serde_json::from_slice::<PublishRequest>(&body) {
        Ok(payload) => payload,
        Err(parse_err) => {
            return err(
                StatusCode::BAD_REQUEST,
                &format!("invalid publish request body: {parse_err}"),
            );
        }
    };
    if payload.history_id == 0 || payload.user_id == 0 {
        return err(StatusCode::BAD_REQUEST, "historyId and userId are required");
    }

    let url = format!("{}/agent/v1/publish", state.upstream_base_url);
    let mut req = state.http_client.post(url).json(&payload);
    if !state.upstream_token.trim().is_empty() {
        req = req
            .bearer_auth(state.upstream_token.trim())
            .header("X-Agent-Token", state.upstream_token.trim());
    }

    match req.send().await {
        Ok(resp) => {
            let status = resp.status();
            let value = resp.json::<serde_json::Value>().await.unwrap_or_else(|_| {
                serde_json::json!({
                    "type": "error",
                    "msg": format!("upstream returned HTTP {}", status.as_u16())
                })
            });
            if !status.is_success() {
                return (
                    StatusCode::BAD_GATEWAY,
                    Json(ApiResponse {
                        response_type: "error",
                        msg: upstream_msg(&value, "upstream publish failed"),
                        data: Some(value),
                    }),
                );
            }
            (
                StatusCode::OK,
                Json(ApiResponse {
                    response_type: "success",
                    msg: upstream_msg(&value, "publish forwarded"),
                    data: Some(value),
                }),
            )
        }
        Err(request_err) => err(
            StatusCode::BAD_GATEWAY,
            &format!("upstream publish request failed: {request_err}"),
        ),
    }
}

async fn check_files_get(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
    Query(params): Query<FileCheckParams>,
) -> (StatusCode, Json<ApiResponse<FileCheckResult>>) {
    check_files_internal(
        state,
        headers,
        FileCheckRequest {
            paths: None,
            limit: params.limit,
            min_size: None,
            extensions: None,
        },
    )
}

async fn check_files_post(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
    body: Bytes,
) -> (StatusCode, Json<ApiResponse<FileCheckResult>>) {
    if let Err(resp) = authorize(&state, &headers) {
        return resp;
    }
    if !state.purpose.allows(CAP_FILESCAN) {
        return err(
            StatusCode::FORBIDDEN,
            "agent purpose does not allow file scan",
        );
    }
    let payload = match serde_json::from_slice::<FileCheckRequest>(&body) {
        Ok(payload) => payload,
        Err(parse_err) => {
            return err(
                StatusCode::BAD_REQUEST,
                &format!("invalid file check request body: {parse_err}"),
            );
        }
    };
    check_files_internal(state, headers, payload)
}

async fn proxy_connect(state: Arc<AppState>, mut req: Request<Incoming>) -> Response<Body> {
    if let Err(resp) = authorize_proxy(&state, req.headers()) {
        return resp;
    }
    if !state.purpose.allows(CAP_UPLOAD) {
        return text_response(
            StatusCode::FORBIDDEN,
            "agent purpose does not allow upload tunnel",
        );
    }
    if req.method() != Method::CONNECT {
        return text_response(
            StatusCode::METHOD_NOT_ALLOWED,
            "agent upload tunnel only supports HTTP CONNECT",
        );
    }

    let Some(authority) = req.uri().authority().map(|v| v.as_str().to_string()) else {
        return text_response(StatusCode::BAD_REQUEST, "CONNECT authority is required");
    };

    info!(target = %authority, "agent upload tunnel CONNECT");
    tokio::spawn(async move {
        match upgrade::on(&mut req).await {
            Ok(upgraded) => {
                let mut client = TokioIo::new(upgraded);
                match TcpStream::connect(&authority).await {
                    Ok(mut server) => match copy_bidirectional(&mut client, &mut server).await {
                        Ok((from_client, from_server)) => {
                            info!(
                                target = %authority,
                                bytes_from_controller = from_client,
                                bytes_from_upstream = from_server,
                                "agent upload tunnel closed"
                            );
                        }
                        Err(err) => {
                            error!(target = %authority, %err, "agent upload tunnel copy failed");
                        }
                    },
                    Err(err) => {
                        error!(target = %authority, %err, "agent upload tunnel connect failed");
                    }
                }
            }
            Err(err) => {
                error!(target = %authority, %err, "agent upload tunnel upgrade failed");
            }
        }
    });

    Response::builder()
        .status(StatusCode::OK)
        .body(Body::empty())
        .unwrap_or_else(|_| text_response(StatusCode::OK, ""))
}

fn check_files_internal(
    state: Arc<AppState>,
    headers: HeaderMap,
    payload: FileCheckRequest,
) -> (StatusCode, Json<ApiResponse<FileCheckResult>>) {
    if let Err(resp) = authorize(&state, &headers) {
        return resp;
    }
    if !state.purpose.allows(CAP_FILESCAN) {
        return err(
            StatusCode::FORBIDDEN,
            "agent purpose does not allow file scan",
        );
    }

    let limit = payload.limit.unwrap_or(100).clamp(1, 1000);
    let min_size = payload.min_size.unwrap_or(1024 * 1024);
    let extensions = normalize_extensions(payload.extensions);
    let paths = payload
        .paths
        .unwrap_or_else(|| vec![state.work_path.display().to_string()]);

    match scan_paths(&paths, min_size, &extensions, limit) {
        Ok(result) => ok(
            "file check completed",
            FileCheckResult {
                purpose: state.purpose.as_str().to_string(),
                work_path: state.work_path.display().to_string(),
                ..result
            },
        ),
        Err(scan_err) => err(StatusCode::OK, &scan_err.to_string()),
    }
}

fn scan_paths(
    paths: &[String],
    min_size: u64,
    extensions: &[String],
    limit: usize,
) -> Result<FileCheckResult> {
    let mut result = FileCheckResult {
        purpose: String::new(),
        work_path: String::new(),
        total_files: 0,
        total_size: 0,
        sample_limit: limit,
        files: Vec::with_capacity(limit),
        errors: Vec::new(),
    };

    if paths.is_empty() {
        bail!("no scan paths configured");
    }

    for raw in paths {
        let path = Path::new(raw.trim());
        if raw.trim().is_empty() {
            continue;
        }
        if !path.exists() {
            result
                .errors
                .push(format!("path not found: {}", path.display()));
            continue;
        }
        for entry in WalkDir::new(path).follow_links(false).into_iter() {
            let entry = match entry {
                Ok(entry) => entry,
                Err(err) => {
                    result.errors.push(err.to_string());
                    continue;
                }
            };
            if !entry.file_type().is_file() {
                continue;
            }
            let file_path = entry.path();
            if !is_allowed_extension(file_path, extensions) {
                continue;
            }
            let metadata = match entry.metadata() {
                Ok(metadata) => metadata,
                Err(err) => {
                    result
                        .errors
                        .push(format!("{}: {err}", file_path.display()));
                    continue;
                }
            };
            if metadata.len() < min_size {
                continue;
            }
            result.total_files += 1;
            result.total_size = result.total_size.saturating_add(metadata.len());
            if result.files.len() < limit {
                result.files.push(FileCheckItem {
                    file_path: file_path.display().to_string(),
                    file_name: file_path
                        .file_name()
                        .map(|v| v.to_string_lossy().to_string())
                        .unwrap_or_default(),
                    file_size: metadata.len(),
                    mod_time: system_time_to_utc(
                        metadata.modified().unwrap_or(SystemTime::UNIX_EPOCH),
                    ),
                });
            }
        }
    }

    Ok(result)
}

fn normalize_extensions(raw: Option<Vec<String>>) -> Vec<String> {
    let values = raw.unwrap_or_else(|| vec!["flv".into(), "mp4".into(), "mkv".into(), "ts".into()]);
    values
        .into_iter()
        .map(|value| value.trim().trim_start_matches('.').to_ascii_lowercase())
        .filter(|value| !value.is_empty())
        .collect()
}

fn is_allowed_extension(path: &Path, extensions: &[String]) -> bool {
    let Some(ext) = path
        .extension()
        .map(|v| v.to_string_lossy().to_ascii_lowercase())
    else {
        return false;
    };
    extensions.iter().any(|allowed| allowed == &ext)
}

fn system_time_to_utc(time: SystemTime) -> DateTime<Utc> {
    DateTime::<Utc>::from(time)
}

fn authorize<T: Serialize>(
    state: &AppState,
    headers: &HeaderMap,
) -> Result<(), (StatusCode, Json<ApiResponse<T>>)> {
    if state.token.trim().is_empty() {
        return Ok(());
    }

    let header_token = headers
        .get("x-agent-token")
        .and_then(|v| v.to_str().ok())
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .or_else(|| {
            headers
                .get("authorization")
                .and_then(|v| v.to_str().ok())
                .map(str::trim)
                .and_then(|auth| {
                    auth.strip_prefix("Bearer ")
                        .or_else(|| auth.strip_prefix("bearer "))
                })
                .map(str::trim)
                .filter(|v| !v.is_empty())
        });

    if header_token == Some(state.token.trim()) {
        Ok(())
    } else {
        Err(err(StatusCode::FORBIDDEN, "invalid agent token"))
    }
}

fn authorize_proxy(state: &AppState, headers: &HeaderMap) -> Result<(), Response<Body>> {
    let token = state.token.trim();
    if token.is_empty() {
        return Ok(());
    }
    if bearer_or_agent_token(headers, token) || proxy_authorization_matches(headers, token) {
        return Ok(());
    }
    Err(text_response(
        StatusCode::PROXY_AUTHENTICATION_REQUIRED,
        "invalid agent proxy token",
    ))
}

fn bearer_or_agent_token(headers: &HeaderMap, token: &str) -> bool {
    headers
        .get("x-agent-token")
        .and_then(|v| v.to_str().ok())
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .is_some_and(|v| v == token)
        || headers
            .get("authorization")
            .and_then(|v| v.to_str().ok())
            .map(str::trim)
            .and_then(|auth| {
                auth.strip_prefix("Bearer ")
                    .or_else(|| auth.strip_prefix("bearer "))
            })
            .map(str::trim)
            .filter(|v| !v.is_empty())
            .is_some_and(|v| v == token)
}

fn proxy_authorization_matches(headers: &HeaderMap, token: &str) -> bool {
    let Some(raw) = headers
        .get("proxy-authorization")
        .and_then(|v| v.to_str().ok())
        .map(str::trim)
    else {
        return false;
    };

    if let Some(value) = raw
        .strip_prefix("Bearer ")
        .or_else(|| raw.strip_prefix("bearer "))
        .map(str::trim)
    {
        return value == token;
    }

    let Some(encoded) = raw
        .strip_prefix("Basic ")
        .or_else(|| raw.strip_prefix("basic "))
        .map(str::trim)
    else {
        return false;
    };
    let Ok(decoded) = general_purpose::STANDARD.decode(encoded) else {
        return false;
    };
    let Ok(value) = String::from_utf8(decoded) else {
        return false;
    };
    let mut parts = value.splitn(2, ':');
    let user = parts.next().unwrap_or_default();
    let password = parts.next().unwrap_or_default();
    user == token || password == token || value == token
}

fn text_response(status: StatusCode, text: &str) -> Response<Body> {
    (status, text.to_string()).into_response()
}

fn ok<T: Serialize>(msg: &str, data: T) -> (StatusCode, Json<ApiResponse<T>>) {
    (
        StatusCode::OK,
        Json(ApiResponse {
            response_type: "success",
            msg: msg.to_string(),
            data: Some(data),
        }),
    )
}

fn err<T: Serialize>(status: StatusCode, msg: &str) -> (StatusCode, Json<ApiResponse<T>>) {
    (
        status,
        Json(ApiResponse {
            response_type: "error",
            msg: msg.to_string(),
            data: None,
        }),
    )
}

fn upstream_msg(value: &serde_json::Value, fallback: &str) -> String {
    value
        .get("msg")
        .or_else(|| value.get("message"))
        .or_else(|| value.get("error"))
        .and_then(|v| v.as_str())
        .unwrap_or(fallback)
        .to_string()
}

fn arg_or_env(args: &[String], flag: &str, env_key: &str) -> Option<String> {
    args.windows(2)
        .find(|window| window[0] == flag)
        .map(|window| window[1].clone())
        .or_else(|| env::var(env_key).ok())
}
