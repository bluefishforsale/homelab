# AI Corporation - Workflow Engine

A FOSS "AI Corporation" workflow engine that uses LLMs as virtual employees (Board, CEO, CTO, Marketing, Artist, Workers) to generate business ideas, evaluate feasibility, and produce artifacts.

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Ansible (for deployment)
- Node.js 20+ (for frontend development)

### Deployment (via Ansible)

```bash
# Add vault secrets first (see vault/secrets.yaml.template)
ansible-vault edit vault/secrets.yaml

# Deploy to ocean server
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/ai_corp.yaml
```

### Local Development (Docker - Recommended)

Build and test in Docker on macOS, then deploy to target architecture:

```bash
cd files/ai-corp/go

# Start full development stack
make docker-up

# Run tests in Docker
make docker-test

# View logs
make docker-logs

# Rebuild and restart after changes
make docker-restart

# Stop everything
make docker-down
```

### Local Development (Native)

**Backend:**
```bash
cd files/ai-corp/go
make deps
make test
make run
```

**Frontend:**
```bash
cd files/ai-corp/web
npm install
npm run dev
```

### Production Build

```bash
cd files/ai-corp/go

# Build for Linux amd64 (typical server)
make docker-build-linux

# Or build multi-arch image (requires buildx)
make docker-build-multi

# Or cross-compile binaries directly
make build-linux       # Linux amd64
make build-linux-arm   # Linux arm64
make build-all         # All platforms
```

## Access URLs

| Service | URL |
|---------|-----|
| API | http://192.168.1.143:8088 |
| Dashboard | http://192.168.1.143:8088 |
| Health Check | http://192.168.1.143:8088/health |
| Metrics | http://192.168.1.143:8088/metrics |

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed system design.

### Core Components

- **Product Pipeline**: Automated product ideation through execution planning lifecycle
- **Workflow Orchestrator**: Executes workflow definitions with step-by-step execution
- **Role Agents**: LLM-backed virtual employees (Board, CEO, CTO, Marketing, Artist, Worker)
- **Provider Abstraction**: Support for Claude 4.5 Sonnet (with extended thinking), Gemini, ChatGPT, and local LLMs (llama.cpp)
- **Extended Thinking**: Claude Sonnet 4.5 uses 5,000 token thinking budget + 3,000 token output (cost-optimized)
- **Job Queue**: Redis-based async workflow processing
- **Persistence**: PostgreSQL for workflow state, history, and seed configuration
- **Organization**: Hierarchical structure with CEO, divisions, departments, managers, employees
- **Scheduler**: Automated task scheduling with SCRUM ceremonies
- **Meeting System**: Full meeting tracking with dialog, decisions, and action items
- **Real-time Updates**: WebSocket broadcasting with frontend polling every 5 seconds

### Technology Stack

| Component | Technology |
|-----------|-----------|
| Backend | Go 1.22 |
| Frontend | React + Vite + TailwindCSS |
| Database | PostgreSQL 16 |
| Cache/Queue | Redis 7 |
| Deployment | Docker Compose + systemd |

## Configuration

Configuration is managed via `config.ini`:

```ini
[server]
port = 8088
log_level = info

[providers]
default = local

[providers.local]
type = openai_compatible
url = http://192.168.1.143:8080
model = default

[roles.ceo]
name = CEO
provider = local
persona = You are the CEO...
```

### Environment Variables

API keys are passed via environment variables:

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | OpenAI API key for GPT-4 |
| `ANTHROPIC_API_KEY` | Anthropic API key for Claude (with extended thinking) |
| `GOOGLE_API_KEY` | Google API key for Gemini |
| `MIDJOURNEY_API_KEY` | Midjourney API key (if enabled) |

**Extended Thinking**: Claude Sonnet 4.5 and newer models automatically enable extended thinking mode with a 5,000 token thinking budget and 8,000 total max tokens (5k thinking + 3k output). Optimized for cost efficiency while maintaining high-quality reasoning.

### Vault Configuration

Add to `vault/secrets.yaml`:

```yaml
ai_services:
  claude:
    api_key: "sk-ant-..."  # Claude with extended thinking
  ai_corp:
    postgres_password: "secure-password"
    # Optional: Add other LLM providers
    # openai_api_key: "sk-..."
    # google_api_key: "AIza..."
```

## Product Pipeline

The system includes an automated product development lifecycle that continuously generates and develops product ideas:

### Pipeline Stages

1. **Ideation** - CEO generates product idea via LLM
2. **Work Packet** - Employees create:
   - Market research
   - Competitive analysis
   - Business plan
   - Financial projections
   - Marketing strategy
3. **C-Suite Review** - Executives review and approve/request revisions (max 3 revisions)
4. **Board Vote** - Simulated board voting on viability
5. **Execution Plan** - Detailed plan with phases, milestones, KPIs, budget
6. **Launched** - Final approved product with downloadable execution plan (HTML)

### Product Generation Constraints

The AI Corporation enforces strict constraints on generated business ideas:

- **Small cap / bootstrappable** - Realistic for startup funding, not requiring massive capital
- **NO AI/ML products** - Absolutely no artificial intelligence, machine learning, or neural network themes
- **NO quantum computing** - No quantum computing, quantum encryption, or any quantum-related products
- **Tangible solutions** - Real products, services, or software that solve practical business problems
- **Startup viable** - Must be executable by a small startup team with limited resources

These constraints are hardcoded into both the initial seeding prompt and the continuous pipeline generation to ensure all product ideas remain practical and bootstrappable.

### Continuous Operation

- System maintains up to **5 concurrent pipelines**
- Automatically generates new ideas every 15 seconds when capacity available
- Persists seed configuration across restarts
- Products diversify based on existing ideas to avoid duplicates

### PDF Export

Launched products include downloadable HTML execution plans that can be:

- **Previewed in browser** - Direct view of formatted plan
- **Downloaded as HTML** - Can be printed to PDF via browser
- **Includes**: Full business plan, execution phases, milestones, KPIs, team structure

## Workflows

Workflows are defined in YAML files in the `workflows/` directory:

```yaml
name: Business Idea Pipeline
description: Generate and evaluate a business idea

inputs:
  - name: domain
    type: string
    required: true

steps:
  - id: ideation
    role: board
    action: generate
    prompt_template: |
      Generate 3 business ideas in {{.domain}}...
    outputs:
      - name: ideas
        type: json

  - id: ceo_select
    role: ceo
    depends_on: [ideation]
    prompt_template: |
      Select the best idea from: {{.ideation.response}}
```

## API Reference

### Workflow APIs

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/api/v1/workflows` | List workflow templates |
| `POST` | `/api/v1/workflows/{id}/run` | Start a workflow |
| `GET` | `/api/v1/runs` | List workflow runs |
| `GET` | `/api/v1/runs/{id}` | Get run details |
| `POST` | `/api/v1/runs/{id}/cancel` | Cancel a run |
| `GET` | `/api/v1/roles` | List configured roles |
| `GET` | `/api/v1/providers` | List LLM providers |

### Organization APIs

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/org/status` | Company status and employee statistics |
| `GET` | `/api/v1/org/structure` | Get full organization structure |
| `GET` | `/api/v1/org/divisions` | List all divisions |
| `GET` | `/api/v1/org/employees` | List all employees |
| `GET` | `/api/v1/org/employees/{id}/detail` | Get detailed employee info with activity log |
| `GET` | `/api/v1/org/person/{id}` | Get person details (employee/manager/exec) |
| `GET` | `/api/v1/org/people` | Search people across org |
| `GET` | `/api/v1/org/deliverables` | List work deliverables with status breakdown |
| `POST` | `/api/v1/org/pause` | Pause company operations |
| `POST` | `/api/v1/org/resume` | Resume company operations |

### Seed APIs

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/org/seed` | Get current company seed configuration |
| `POST` | `/api/v1/org/seed` | Set company seed (name, sector, mission, vision) |
| `GET` | `/api/v1/org/sectors` | List available business sectors |

### Product Pipeline APIs

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/org/products` | List old product ideas (legacy) |
| `GET` | `/api/v1/org/pipelines` | List all product pipelines with stage info |
| `GET` | `/api/v1/org/pipelines/{id}/download` | Download execution plan HTML (preview in browser) |
| `GET` | `/api/v1/org/pipelines/{id}/download?download=true` | Download execution plan HTML (force download) |

### Meeting APIs

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/meetings` | List all meetings |
| `GET` | `/api/v1/meetings/{id}` | Get meeting details with dialog and decisions |

## Web Dashboard Pages

| Route | Page | Description |
|-------|------|-------------|
| `/` | Dashboard | Company status, product pipeline summary, active workers |
| `/products` | Products | Product pipeline viewer with stage breakdown and downloads |
| `/seed` | Seed Setup | Configure and seed the AI corporation |
| `/workflows` | Workflows | View and launch workflow templates |
| `/runs` | Runs | Monitor active and completed workflow runs |
| `/runs/:id` | Run Detail | Detailed view of a specific run |
| `/org` | Organization | Interactive org chart with divisions, departments, employees |
| `/org/employee/:id` | Employee Detail | Individual employee page with activity log, work history |
| `/meetings` | Meetings | List of all SCRUM and board meetings |
| `/meetings/:id` | Meeting Detail | Full meeting transcript with dialog, decisions, action items |
| `/people` | People | Search and browse all people in org |
| `/admin` | Admin | System settings, reset, configuration |

## SCRUM Meetings

The AI Corporation runs on SCRUM methodology with automated recurring meetings:

| Meeting | Schedule | Duration | Description |
|---------|----------|----------|-------------|
| Daily Standup | Daily 9:00 AM | 15 min | What did/doing/blockers |
| Sprint Planning | Monday 10:00 AM | 2 hours | Select sprint backlog |
| Sprint Review | Friday 2:00 PM | 1 hour | Demo completed work |
| Sprint Retrospective | Friday 3:30 PM | 1 hour | What went well/improve |
| Board Meeting | Monday 9:00 AM | 2 hours | Weekly project review |
| Quarterly Strategy | Quarterly | 8 hours | Strategic planning |

Meetings are automatically generated with:

- Full dialog transcripts with speakers and timestamps
- Attendee list from organization structure
- Decisions with voting records
- Action items for follow-up
- Meeting summaries

## Monitoring

Prometheus metrics available at `/metrics`:

- `aicorp_workflows_started_total` - Workflows started
- `aicorp_workflows_completed_total` - Workflows completed
- `aicorp_llm_requests_total` - LLM API requests
- `aicorp_llm_latency_seconds` - LLM request latency
- `aicorp_llm_tokens_input_total` - Input tokens used
- `aicorp_llm_tokens_output_total` - Output tokens generated
- `aicorp_active_workflows` - Currently running workflows
- `aicorp_queue_depth` - Jobs in queue

## Development

### Adding a New Provider

1. Create provider struct implementing the `Provider` interface in `providers.go`
2. Add initialization in `NewProviderManager()`
3. Add config section `[providers.yourprovider]`

### Adding a New Role

Add to `config.ini`:

```ini
[roles.newrole]
name = New Role Name
provider = local
persona = Your persona description...
```

### Adding a New Workflow

Create YAML file in `workflows/`:

```yaml
name: My Workflow
description: What it does
version: "1.0"

inputs:
  - name: input1
    type: string
    required: true

steps:
  - id: step1
    role: worker
    prompt_template: |
      Do something with {{.input1}}
```

## Troubleshooting

**Service won't start:**
```bash
# Check logs
journalctl -u ai-corp -f

# Check container status
docker compose ps
docker compose logs ai-corp
```

**Database connection issues:**
```bash
# Check PostgreSQL
docker compose exec postgres psql -U ai_corp -c '\dt'
```

**LLM not responding:**
```bash
# Test provider
curl -X POST http://192.168.1.143:8088/api/v1/providers/local/test
```

## License

MIT
