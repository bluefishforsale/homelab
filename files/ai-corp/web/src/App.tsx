import { useState, useEffect, useCallback, useRef, createContext, useContext } from 'react'
import { Routes, Route, Link, useLocation, useParams, useNavigate } from 'react-router-dom'
import { 
  Home, 
  Play, 
  Pause,
  Users, 
  Settings, 
  Activity,
  CheckCircle,
  XCircle,
  Clock,
  Loader2,
  Building2,
  UserCircle,
  BarChart3,
  Edit3,
  Save,
  RefreshCw,
  Rocket,
  Target,
  ChevronLeft,
  FileText
} from 'lucide-react'

// Types
interface HealthResponse {
  status: string
  mode: string
  service: string
  uptime_seconds: number
  active_runs: number
  queue_depth: number
  checks: Record<string, { status: string; latency_ms?: number; error?: string }>
}

interface WorkflowTemplate {
  id: string
  name: string
  description: string
  version: string
}

interface WorkflowRun {
  id: string
  template_id: string
  status: string
  inputs: Record<string, unknown>
  outputs: Record<string, unknown>
  error?: string
  started_at?: string
  completed_at?: string
  created_at: string
  steps?: StepExecution[]
}

interface StepExecution {
  id: string
  step_id: string
  role: string
  status: string
  prompt?: string
  response?: string
  tokens_used: number
  latency_ms: number
  error?: string
}

interface OrgStats {
  divisions: number
  managers: number
  total_employees: number
  by_status: Record<string, number>
  by_skill: Record<string, number>
}

interface CompanyStatus {
  status: 'running' | 'paused' | 'stopped'
  stats: OrgStats
}

interface Person {
  id: string
  type: string
  name: string
  role: string
  status?: string
  style?: string
}

interface Biography {
  id?: string
  person_id: string
  person_type: string
  name: string
  bio: string
  background: string
  personality: string
  goals: string[]
  values: string[]
  quirks: string[]
  exists?: boolean
}

interface SectorInfo {
  id: string
  name: string
  description: string
  examples: string[]
}

interface CompanySeed {
  id?: string
  sector: string
  custom_sector?: string
  company_name: string
  mission: string
  vision: string
  target_market: string
  initial_budget?: number
  constraints?: string[]
  goals?: string[]
  active: boolean
  created_at?: string
}

interface Division {
  id: string
  name: string
  description: string
  departments: number
  created_at: string
}

interface Employee {
  id: string
  name: string
  skill: string
  status: string
  manager_id?: string
  manager_name?: string
  work_count: number
  title?: string
  current_work?: string
  direct_reports?: string[]
  expectations?: string[]
}

interface PersonDetail {
  id: string
  type: 'employee' | 'manager' | 'department_head' | 'board_member'
  name: string
  title: string
  boss?: { id: string; name: string; title: string }
  direct_reports: { id: string; name: string; title: string }[]
  expectations: string[]
  skill?: string
  status?: string
  style?: string
}

interface BoardMember {
  id: string
  name: string
  title: string
  background: string
  expertise: string[]
  voting_style: string
}

interface SystemStatus {
  database: boolean
  redis: boolean
  storage: boolean
  providers: boolean
  organization: boolean
  provider_count?: number
  default_provider?: string
  org_status?: string
  org_stats?: OrgStats
  seeded?: boolean
  seed?: CompanySeed
  config?: {
    server_port: number
    websocket_port: number
    workflow_dir: string
  }
}

interface WorkItem {
  id: string
  type: string
  title: string
  description: string
  objectives: string[]
  priority: number
  created_at: string
}

interface WorkResult {
  id: string
  work_item_id: string
  output: string
  tokens_used: number
  completed_at: string
  duration: number
}

interface ProductIdea {
  id: string
  name: string
  description: string
  category: string
  status: 'ideation' | 'planning' | 'development' | 'review' | 'approved' | 'launched' | 'rejected'
  target_market: string
  value_prop: string
  features: string[]
  created_at: string
  updated_at: string
}

interface ProductsResponse {
  products: ProductIdea[]
  total: number
  by_status: {
    ideation: number
    planning: number
    development: number
    review: number
    approved: number
    launched: number
    rejected: number
  }
}

interface Pipeline {
  id: string
  name: string
  description: string
  category: string
  stage: 'ideation' | 'work_packet' | 'csuite_review' | 'board_vote' | 'execution_plan' | 'production' | 'final_review' | 'launched' | 'rejected'
  target_market: string
  revision_count: number
  created_at: string
  updated_at: string
  idea?: {
    problem: string
    solution: string
    value_proposition: string
    target_customer: string
    revenue_model: string
  }
  has_work_packet?: boolean
  csuite_review?: {
    approved: boolean
    feedback: string
  }
  board_decision?: {
    approved: boolean
    votes_for: number
    votes_against: number
  }
  execution_plan?: {
    timeline: string
    budget: string
  }
}

interface PipelinesResponse {
  pipelines: Pipeline[]
  total: number
  by_stage: Record<string, number>
}

interface Deliverable {
  id: string
  title: string
  type: string
  description: string
  output: string
  status: 'in_progress' | 'completed' | 'in_review' | 'approved' | 'rejected'
  employee_id: string
  employee_name: string
  skill: string
  created_at: string
  completed_at?: string
  duration_ms: number
}

interface DeliverablesResponse {
  deliverables: Deliverable[]
  total: number
  by_status: {
    completed: number
    in_progress: number
    in_review: number
    approved: number
    rejected: number
  }
}

interface ActivityLogEntry {
  id: string
  timestamp: string
  type: string
  title: string
  description: string
  work_item_id?: string
  metadata?: Record<string, unknown>
}

interface EmployeeDetail {
  id: string
  name: string
  skill: string
  status: string
  hired_at: string
  persona: string
  manager?: { id: string; name: string }
  work_count: number
  current_work?: WorkItem
  time_on_task?: string
  activity_log: ActivityLogEntry[]
  work_history: WorkResult[]
  stats: {
    completed_tasks: number
    total_work_time: string
    average_task_time: string
    activity_count: number
  }
}

interface MeetingDialogEntry {
  id: string
  timestamp: string
  speaker: string
  speaker_id: string
  role: string
  content: string
  type: string
}

interface BoardDecision {
  id: string
  type: string
  subject: string
  description: string
  proposed_by: string
  votes: { member_id: string; vote: string; reasoning: string }[]
  passed: boolean
  pass_pct: number
  decision: string
  summary: string
}

interface MeetingSummary {
  id: string
  type: string
  title: string
  scheduled_at: string
  started_at?: string
  ended_at?: string
  status: string
  decision_count: number
  dialog_count: number
  attendee_count: number
  summary?: string
}

interface Meeting {
  id: string
  type: string
  title: string
  scheduled_at: string
  started_at?: string
  ended_at?: string
  status: string
  agenda: { id: string; type: string; title: string; description: string; presenter: string }[]
  dialog: MeetingDialogEntry[]
  decisions: BoardDecision[]
  summary: string
  key_decisions: string[]
  action_items: string[]
  attendees: string[]
}

// API functions
const api = {
  async getHealth(): Promise<HealthResponse> {
    const res = await fetch('/health')
    return res.json()
  },
  async getWorkflows(): Promise<{ workflows: WorkflowTemplate[] }> {
    const res = await fetch('/api/v1/workflows')
    return res.json()
  },
  async getRuns(status?: string): Promise<{ runs: WorkflowRun[] }> {
    const url = status ? `/api/v1/runs?status=${status}` : '/api/v1/runs'
    const res = await fetch(url)
    return res.json()
  },
  async getRun(id: string): Promise<WorkflowRun> {
    const res = await fetch(`/api/v1/runs/${id}`)
    return res.json()
  },
  async startRun(templateId: string, inputs: Record<string, unknown>): Promise<WorkflowRun> {
    const res = await fetch(`/api/v1/workflows/${templateId}/run`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ inputs })
    })
    return res.json()
  },
  async cancelRun(id: string): Promise<void> {
    await fetch(`/api/v1/runs/${id}/cancel`, { method: 'POST' })
  },
  // Organization APIs
  async getOrgStatus(): Promise<CompanyStatus> {
    const res = await fetch('/api/v1/org/status')
    return res.json()
  },
  async getOrgStats(): Promise<OrgStats> {
    const res = await fetch('/api/v1/org/stats')
    return res.json()
  },
  async getPersonDetail(id: string): Promise<PersonDetail> {
    const res = await fetch(`/api/v1/org/person/${id}`)
    return res.json()
  },
  async getEmployeeDetail(id: string): Promise<EmployeeDetail> {
    const res = await fetch(`/api/v1/org/employees/${id}/detail`)
    return res.json()
  },
  async listMeetings(): Promise<{ meetings: MeetingSummary[]; total: number }> {
    const res = await fetch('/api/v1/meetings')
    return res.json()
  },
  async getMeeting(id: string): Promise<Meeting> {
    const res = await fetch(`/api/v1/meetings/${id}`)
    return res.json()
  },
  async pauseCompany(): Promise<void> {
    await fetch('/api/v1/org/pause', { method: 'POST' })
  },
  async resumeCompany(): Promise<void> {
    await fetch('/api/v1/org/resume', { method: 'POST' })
  },
  async getDivisions(): Promise<{ divisions: Division[], total: number }> {
    const res = await fetch('/api/v1/org/divisions')
    return res.json()
  },
  async getEmployees(skill?: string, status?: string): Promise<{ employees: Employee[], total: number }> {
    let url = '/api/v1/org/employees'
    const params = new URLSearchParams()
    if (skill) params.append('skill', skill)
    if (status) params.append('status', status)
    if (params.toString()) url += '?' + params.toString()
    const res = await fetch(url)
    return res.json()
  },
  async getPeople(): Promise<{ people: Person[], total: number }> {
    const res = await fetch('/api/v1/org/people')
    return res.json()
  },
  async getBoardMembers(): Promise<{ members: BoardMember[], total: number }> {
    const res = await fetch('/api/v1/board/members')
    return res.json()
  },
  // Biography APIs
  async getBiography(personId: string): Promise<Biography> {
    const res = await fetch(`/api/v1/biographies/${personId}`)
    return res.json()
  },
  async updateBiography(bio: Biography): Promise<void> {
    await fetch('/api/v1/biographies', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(bio)
    })
  },
  // Seed/Bootstrap APIs
  async getSectors(): Promise<{ sectors: SectorInfo[], total: number }> {
    const res = await fetch('/api/v1/org/sectors')
    return res.json()
  },
  async getSeed(): Promise<{ seeded: boolean, seed: CompanySeed | null }> {
    const res = await fetch('/api/v1/org/seed')
    return res.json()
  },
  async setSeed(seed: Partial<CompanySeed>): Promise<{ status: string, seed: CompanySeed }> {
    const res = await fetch('/api/v1/org/seed', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(seed)
    })
    return res.json()
  },
  // Admin APIs
  async getSystemStatus(): Promise<SystemStatus> {
    const res = await fetch('/api/v1/admin/status')
    return res.json()
  },
  async resetOrganization(): Promise<{ status: string, message: string }> {
    const res = await fetch('/api/v1/admin/reset', { method: 'POST' })
    return res.json()
  },
  // Deliverables API
  async getDeliverables(status?: string): Promise<DeliverablesResponse> {
    const params = status ? `?status=${status}` : ''
    const res = await fetch(`/api/v1/org/deliverables${params}`)
    return res.json()
  },
  // Products API
  async getProducts(): Promise<ProductsResponse> {
    const res = await fetch('/api/v1/org/products')
    return res.json()
  },
  // Pipelines API
  async getPipelines(): Promise<PipelinesResponse> {
    const res = await fetch('/api/v1/org/pipelines')
    return res.json()
  },
}

// WebSocket message types
interface WSMessage {
  type: 'org_update' | 'employee_update' | 'work_complete' | 'seed_update' | 'work_assigned' | 'work_started' | 'work_failed'
  payload: unknown
  timestamp?: string
}

// WebSocket context for sharing live updates
interface WSContextType {
  connected: boolean
  lastMessage: WSMessage | null
  orgStats: OrgStats | null
  activityLog: ActivityLogEntry[]
}

const WSContext = createContext<WSContextType>({
  connected: false,
  lastMessage: null,
  orgStats: null,
  activityLog: []
})

// Simple UUID generator for activity log IDs (fallback for browsers without crypto.randomUUID)
function generateId(): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => {
    const r = Math.random() * 16 | 0
    return (c === 'x' ? r : (r & 0x3 | 0x8)).toString(16)
  })
}

// Helper to format activity message from WebSocket payload
function formatActivityMessage(type: string, payload: unknown): string {
  const p = payload as Record<string, unknown>
  switch (type) {
    case 'employee_update':
      return `${p.employee_name || 'Employee'} status: ${p.status || 'updated'}`
    case 'work_complete':
      return `${p.employee_name || 'Employee'} completed: ${p.work_title || 'work'} ${p.has_error ? '(with errors)' : ''}`
    case 'work_assigned':
      return `Work assigned: ${p.work_title || 'task'} to ${p.employee_name || 'employee'}`
    case 'work_started':
      return `${p.employee_name || 'Employee'} started: ${p.work_title || 'work'}`
    case 'work_failed':
      return `${p.employee_name || 'Employee'} failed: ${p.work_title || 'work'} - ${p.error || 'unknown error'}`
    case 'seed_update':
      return `Company seed updated: ${p.company_name || 'company'}`
    case 'org_update':
      return `Org stats: ${p.total_employees || 0} employees, ${(p.by_status as Record<string,number>)?.working || 0} working`
    default:
      return `${type}: ${JSON.stringify(payload).slice(0, 50)}`
  }
}

// Custom hook for WebSocket connection
function useWebSocket() {
  const [connected, setConnected] = useState(false)
  const [lastMessage, setLastMessage] = useState<WSMessage | null>(null)
  const [orgStats, setOrgStats] = useState<OrgStats | null>(null)
  const [activityLog, setActivityLog] = useState<ActivityLogEntry[]>([])
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const connect = useCallback(() => {
    // Determine WebSocket URL based on current location
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/api/v1/ws`
    
    try {
      const ws = new WebSocket(wsUrl)
      wsRef.current = ws

      ws.onopen = () => {
        console.log('WebSocket connected')
        setConnected(true)
        // Add connection event to activity log
        setActivityLog(prev => [{
          id: generateId(),
          timestamp: new Date().toISOString(),
          type: 'system',
          title: 'Connected',
          description: 'WebSocket connected to server'
        }, ...prev].slice(0, 100))
      }

      ws.onmessage = (event) => {
        try {
          const msg: WSMessage = JSON.parse(event.data)
          setLastMessage(msg)
          
          // Handle org_update to update stats in real-time
          if (msg.type === 'org_update' && msg.payload) {
            setOrgStats(msg.payload as OrgStats)
          }
          
          // Add to activity log (skip frequent org_update to reduce noise)
          if (msg.type !== 'org_update') {
            const p = msg.payload as Record<string, unknown>
            setActivityLog(prev => [{
              id: generateId(),
              timestamp: new Date().toISOString(),
              type: msg.type,
              title: formatActivityMessage(msg.type, msg.payload),
              description: p.work_title as string || p.employee_name as string || msg.type,
              metadata: p
            }, ...prev].slice(0, 100)) // Keep last 100 entries
          }
        } catch (e) {
          console.error('Failed to parse WebSocket message:', e)
        }
      }

      ws.onclose = () => {
        console.log('WebSocket disconnected')
        setConnected(false)
        wsRef.current = null
        
        // Reconnect after 3 seconds
        reconnectTimeoutRef.current = setTimeout(() => {
          connect()
        }, 3000)
      }

      ws.onerror = (error) => {
        console.error('WebSocket error:', error)
        ws.close()
      }
    } catch (error) {
      console.error('Failed to create WebSocket:', error)
      // Retry connection after 3 seconds
      reconnectTimeoutRef.current = setTimeout(() => {
        connect()
      }, 3000)
    }
  }, [])

  useEffect(() => {
    connect()

    return () => {
      if (wsRef.current) {
        wsRef.current.close()
      }
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current)
      }
    }
  }, [connect])

  return { connected, lastMessage, orgStats, activityLog }
}

// WebSocket provider component
function WSProvider({ children }: { children: React.ReactNode }) {
  const wsState = useWebSocket()
  return <WSContext.Provider value={wsState}>{children}</WSContext.Provider>
}

// Hook to use WebSocket context
function useWS() {
  return useContext(WSContext)
}

// Status badge component
function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    pending: 'bg-yellow-100 text-yellow-800',
    running: 'bg-blue-100 text-blue-800',
    completed: 'bg-green-100 text-green-800',
    failed: 'bg-red-100 text-red-800',
    cancelled: 'bg-gray-100 text-gray-800',
  }
  const icons: Record<string, React.ReactNode> = {
    pending: <Clock className="w-3 h-3" />,
    running: <Loader2 className="w-3 h-3 animate-spin" />,
    completed: <CheckCircle className="w-3 h-3" />,
    failed: <XCircle className="w-3 h-3" />,
    cancelled: <XCircle className="w-3 h-3" />,
  }
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-1 rounded-full text-xs font-medium ${colors[status] || 'bg-gray-100'}`}>
      {icons[status]}
      {status}
    </span>
  )
}

// Dashboard page with multiple panes
function Dashboard() {
  const { lastMessage, activityLog, orgStats } = useWS()
  const [health, setHealth] = useState<HealthResponse | null>(null)
  const [companyStatus, setCompanyStatus] = useState<CompanyStatus | null>(null)
  const [divisions, setDivisions] = useState<Division[]>([])
  const [employees, setEmployees] = useState<Employee[]>([])
  const [people, setPeople] = useState<Person[]>([])
  const [deliverables, setDeliverables] = useState<Deliverable[]>([])
  const [deliverableStats, setDeliverableStats] = useState<DeliverablesResponse['by_status'] | null>(null)
  const [products, setProducts] = useState<ProductIdea[]>([])
  const [productStats, setProductStats] = useState<ProductsResponse['by_status'] | null>(null)
  const [seed, setSeed] = useState<CompanySeed | null>(null)
  const [selectedPerson, setSelectedPerson] = useState<Person | null>(null)
  const [biography, setBiography] = useState<Biography | null>(null)
  const [bioEditing, setBioEditing] = useState(false)
  const [selectedDeliverable, setSelectedDeliverable] = useState<Deliverable | null>(null)
  const [actionLoading, setActionLoading] = useState(false)

  // Fetch data independently - don't block UI on slow endpoints
  useEffect(() => {
    
    // Fetch each data source independently with timeouts
    const controller = new AbortController()
    const timeout = 5000 // 5 second timeout per request
    
    const fetchWithTimeout = async <T,>(fetcher: () => Promise<T>, fallback: T): Promise<T> => {
      try {
        const timeoutId = setTimeout(() => controller.abort(), timeout)
        const result = await fetcher()
        clearTimeout(timeoutId)
        return result
      } catch {
        return fallback
      }
    }
    
    const fetchAll = () => {
      // Fire off all requests independently
      fetchWithTimeout(() => api.getHealth(), null).then(data => data && setHealth(data))
      fetchWithTimeout(() => api.getOrgStatus(), null).then(data => data && setCompanyStatus(data))
      fetchWithTimeout(() => api.getSeed(), { seeded: false, seed: null }).then(data => {
        if (data.seeded && data.seed) {
          setSeed(data.seed)
        }
      })
      fetchWithTimeout(() => api.getDivisions(), { divisions: [], total: 0 }).then(data => setDivisions(data.divisions || []))
      fetchWithTimeout(() => api.getEmployees(), { employees: [], total: 0 }).then(data => setEmployees(data.employees || []))
      fetchWithTimeout(() => api.getPeople(), { people: [], total: 0 }).then(data => setPeople(data.people || []))
      fetchWithTimeout(() => api.getDeliverables(), { deliverables: [], total: 0, by_status: { completed: 0, in_progress: 0, in_review: 0, approved: 0, rejected: 0 } }).then(data => {
        setDeliverables(data.deliverables || [])
        setDeliverableStats(data.by_status)
      })
      fetchWithTimeout(() => api.getProducts(), { products: [], total: 0, by_status: { ideation: 0, planning: 0, development: 0, review: 0, approved: 0, launched: 0, rejected: 0 } }).then(data => {
        setProducts(data.products || [])
        setProductStats(data.by_status)
      })
    }
    
    // Initial fetch
    fetchAll()
    
    // Poll every 5 seconds
    const interval = setInterval(fetchAll, 5000)
    
    return () => {
      controller.abort()
      clearInterval(interval)
    }
  }, [])

  // Update employees and deliverables when we get WebSocket messages
  useEffect(() => {
    if (lastMessage?.type === 'employee_update' || lastMessage?.type === 'work_complete') {
      // Refresh employees list on work updates
      api.getEmployees().then(data => setEmployees(data.employees || []))
    }
    if (lastMessage?.type === 'work_complete') {
      // Refresh deliverables when work completes
      api.getDeliverables().then(data => {
        setDeliverables(data.deliverables || [])
        setDeliverableStats(data.by_status)
      })
    }
  }, [lastMessage])

  const handleToggleCompany = async () => {
    setActionLoading(true)
    try {
      if (companyStatus?.status === 'running') {
        await api.pauseCompany()
      } else {
        await api.resumeCompany()
      }
      const status = await api.getOrgStatus()
      setCompanyStatus(status)
    } finally {
      setActionLoading(false)
    }
  }

  const handleSelectPerson = async (person: Person) => {
    setSelectedPerson(person)
    try {
      const bio = await api.getBiography(person.id)
      setBiography(bio)
    } catch {
      setBiography({
        person_id: person.id,
        person_type: person.type,
        name: person.name,
        bio: '',
        background: '',
        personality: '',
        goals: [],
        values: [],
        quirks: []
      })
    }
    setBioEditing(false)
  }

  const handleSaveBiography = async () => {
    if (!biography) return
    setActionLoading(true)
    try {
      await api.updateBiography(biography)
      setBioEditing(false)
    } finally {
      setActionLoading(false)
    }
  }

  // Calculate metrics
  const workingEmployees = employees.filter(e => e.status === 'working').length

  return (
    <div className="space-y-4">
      {/* Compact Header with Status and Controls */}
      <div className="bg-white rounded-lg shadow p-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-6">
            <h1 className="text-xl font-bold">{seed ? seed.company_name : 'AI Corporation'}</h1>
            <div className="flex items-center gap-4 text-sm">
              <span className="flex items-center gap-2">
                <span className={`w-2 h-2 rounded-full ${companyStatus?.status === 'running' ? 'bg-green-500 animate-pulse' : 'bg-yellow-500'}`} />
                <span className="font-medium capitalize">{companyStatus?.status || 'Unknown'}</span>
              </span>
              <span className="text-gray-400">|</span>
              <span className="flex items-center gap-1">
                <Users className="w-4 h-4 text-gray-400" />
                <span className="text-green-600 font-medium">{workingEmployees}</span>
                <span className="text-gray-400">/</span>
                <span>{employees.length}</span>
                <span className="text-gray-500 text-xs">working</span>
              </span>
              <span className="text-gray-400">|</span>
              <span className="text-gray-500">Queue: {health?.queue_depth || 0}</span>
              <span className="text-gray-500">Uptime: {Math.floor((health?.uptime_seconds || 0) / 3600)}h</span>
            </div>
          </div>
          <button
            onClick={handleToggleCompany}
            disabled={actionLoading}
            className={`flex items-center gap-2 px-4 py-2 rounded-lg font-medium text-white transition-colors ${
              companyStatus?.status === 'running' 
                ? 'bg-red-500 hover:bg-red-600' 
                : 'bg-green-500 hover:bg-green-600'
            } disabled:opacity-50`}
          >
            {actionLoading ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : companyStatus?.status === 'running' ? (
              <><Pause className="w-4 h-4" /> Pause</>
            ) : (
              <><Play className="w-4 h-4" /> Resume</>
            )}
          </button>
        </div>
      </div>

      {/* Product Pipeline - Main Focus */}
      <div className="bg-white rounded-lg shadow p-4">
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2">
            <Rocket className="w-5 h-5 text-purple-600" />
            <h2 className="text-lg font-semibold">Product Pipeline</h2>
          </div>
          <Link to="/products" className="text-sm text-blue-600 hover:underline">View All →</Link>
        </div>
        
        {/* Pipeline stages */}
        <div className="grid grid-cols-4 gap-2 mb-4">
          <div className="bg-purple-50 rounded p-3 text-center">
            <div className="text-2xl font-bold text-purple-600">{productStats?.ideation || 0}</div>
            <div className="text-xs text-purple-700">Ideation</div>
          </div>
          <div className="bg-blue-50 rounded p-3 text-center">
            <div className="text-2xl font-bold text-blue-600">{productStats?.development || 0}</div>
            <div className="text-xs text-blue-700">Development</div>
          </div>
          <div className="bg-yellow-50 rounded p-3 text-center">
            <div className="text-2xl font-bold text-yellow-600">{productStats?.review || 0}</div>
            <div className="text-xs text-yellow-700">Review</div>
          </div>
          <div className="bg-green-50 rounded p-3 text-center">
            <div className="text-2xl font-bold text-green-600">{(productStats?.approved || 0) + (productStats?.launched || 0)}</div>
            <div className="text-xs text-green-700">Launched</div>
          </div>
        </div>

        {products.length === 0 ? (
          <div className="p-4 text-center text-gray-500 border rounded">
            {seed ? (
              <>Products are being generated for <strong>{seed.company_name}</strong>. Check back in a moment...</>
            ) : (
              <>No products yet. <Link to="/seed" className="text-blue-600 hover:underline">Seed the company</Link> to start generating ideas.</>
            )}
          </div>
        ) : (
          <div className="border rounded divide-y max-h-48 overflow-y-auto">
            {products.slice(0, 5).map(p => (
              <div key={p.id} className="p-3 flex items-center justify-between">
                <div>
                  <div className="font-medium text-sm">{p.name}</div>
                  <div className="text-xs text-gray-500">{p.category}</div>
                </div>
                <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                  p.status === 'approved' || p.status === 'launched' ? 'bg-green-100 text-green-700' :
                  p.status === 'development' ? 'bg-blue-100 text-blue-700' :
                  p.status === 'review' ? 'bg-yellow-100 text-yellow-700' :
                  p.status === 'rejected' ? 'bg-red-100 text-red-700' :
                  'bg-purple-100 text-purple-700'
                }`}>
                  {p.status}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Two Column Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        
        {/* Work Deliverables */}
        <div className="bg-white rounded-lg shadow p-4">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <FileText className="w-5 h-5 text-indigo-600" />
              <h2 className="font-semibold">Work Deliverables</h2>
            </div>
            <span className="text-sm text-gray-500">{deliverables.length} total</span>
          </div>
          
          <div className="grid grid-cols-5 gap-1 mb-3">
            <div className="bg-green-50 rounded p-2 text-center">
              <div className="text-lg font-bold text-green-600">{deliverableStats?.completed || 0}</div>
              <div className="text-[9px] text-green-700">Done</div>
            </div>
            <div className="bg-blue-50 rounded p-2 text-center">
              <div className="text-lg font-bold text-blue-600">{deliverableStats?.in_review || 0}</div>
              <div className="text-[9px] text-blue-700">Review</div>
            </div>
            <div className="bg-emerald-50 rounded p-2 text-center">
              <div className="text-lg font-bold text-emerald-600">{deliverableStats?.approved || 0}</div>
              <div className="text-[9px] text-emerald-700">OK</div>
            </div>
            <div className="bg-red-50 rounded p-2 text-center">
              <div className="text-lg font-bold text-red-600">{deliverableStats?.rejected || 0}</div>
              <div className="text-[9px] text-red-700">Fail</div>
            </div>
            <div className="bg-yellow-50 rounded p-2 text-center">
              <div className="text-lg font-bold text-yellow-600">{deliverableStats?.in_progress || 0}</div>
              <div className="text-[9px] text-yellow-700">WIP</div>
            </div>
          </div>

          <div className="border rounded divide-y max-h-48 overflow-y-auto">
            {deliverables.length === 0 ? (
              <div className="p-4 text-center text-gray-500 text-sm">No deliverables yet</div>
            ) : (
              deliverables.slice(0, 5).map(d => (
                <div 
                  key={d.id} 
                  className={`p-2 cursor-pointer hover:bg-gray-50 ${selectedDeliverable?.id === d.id ? 'bg-blue-50' : ''}`}
                  onClick={() => setSelectedDeliverable(selectedDeliverable?.id === d.id ? null : d)}
                >
                  <div className="flex items-center justify-between">
                    <div className="flex-1 min-w-0">
                      <div className="font-medium text-sm truncate">{d.title}</div>
                      <div className="text-xs text-gray-500">{d.employee_name}</div>
                    </div>
                    <span className={`ml-2 px-2 py-0.5 rounded text-[10px] font-medium ${
                      d.status === 'completed' ? 'bg-green-100 text-green-700' :
                      d.status === 'approved' ? 'bg-emerald-100 text-emerald-700' :
                      d.status === 'rejected' ? 'bg-red-100 text-red-700' :
                      'bg-yellow-100 text-yellow-700'
                    }`}>
                      {d.status.replace('_', ' ')}
                    </span>
                  </div>
                  {selectedDeliverable?.id === d.id && (
                    <div className="mt-2 p-2 bg-gray-50 rounded text-xs">
                      <div className="whitespace-pre-wrap text-gray-600 max-h-24 overflow-y-auto">{d.output || 'No output'}</div>
                    </div>
                  )}
                </div>
              ))
            )}
          </div>
        </div>

        {/* Working Employees */}
        <div className="bg-white rounded-lg shadow p-4">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <Users className="w-5 h-5 text-blue-600" />
              <h2 className="font-semibold">Active Workers</h2>
            </div>
            <span className="text-sm text-gray-500">{workingEmployees} active</span>
          </div>
          
          <div className="border rounded divide-y max-h-64 overflow-y-auto">
            {employees.filter(e => e.status === 'working').length === 0 ? (
              <div className="p-4 text-center text-gray-500 text-sm">No employees currently working</div>
            ) : (
              employees.filter(e => e.status === 'working').map(emp => (
                <div key={emp.id} className="p-3">
                  <div className="flex items-center justify-between">
                    <div>
                      <div className="font-medium text-sm">{emp.name}</div>
                      <div className="text-xs text-gray-500 capitalize">{emp.skill}</div>
                    </div>
                    <span className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
                  </div>
                  {emp.current_work && (
                    <div className="mt-1 text-xs text-blue-600 truncate">
                      Working on: {emp.current_work}
                    </div>
                  )}
                </div>
              ))
            )}
          </div>
        </div>

        {/* Corporate Structure Pane */}
        <div className="bg-white rounded-lg shadow">
          <div className="px-4 py-3 border-b flex items-center gap-2">
            <Building2 className="w-5 h-5 text-purple-600" />
            <h2 className="font-semibold">Corporate Structure</h2>
          </div>
          <div className="p-4 max-h-64 overflow-y-auto">
            {divisions.length === 0 ? (
              <div className="text-center text-gray-500">No divisions found</div>
            ) : (
              <div className="space-y-3">
                {divisions.map(div => (
                  <div key={div.id} className="bg-gray-50 rounded p-3">
                    <div className="font-medium">{div.name}</div>
                    <div className="text-xs text-gray-500">{div.description}</div>
                    <div className="text-xs text-primary-600 mt-1">{div.departments} departments</div>
                  </div>
                ))}
              </div>
            )}
            <div className="mt-4 pt-3 border-t">
              <div className="text-sm font-medium mb-2">Employee Distribution</div>
              <div className="grid grid-cols-2 gap-2 text-xs">
                {Object.entries(companyStatus?.stats?.by_skill || {}).map(([skill, count]) => (
                  <div key={skill} className="flex justify-between bg-gray-50 rounded p-2">
                    <span className="capitalize">{skill}</span>
                    <span className="font-medium">{count as number}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>

        {/* Employee Details with Biography Pane */}
        <div className="bg-white rounded-lg shadow">
          <div className="px-4 py-3 border-b flex items-center gap-2">
            <UserCircle className="w-5 h-5 text-orange-600" />
            <h2 className="font-semibold">Employee Details & Biography</h2>
          </div>
          <div className="grid grid-cols-3 divide-x h-80">
            {/* People list */}
            <div className="overflow-y-auto">
              {people.map(person => (
                <button
                  key={person.id}
                  onClick={() => handleSelectPerson(person)}
                  className={`w-full text-left p-2 text-xs hover:bg-gray-50 border-b ${
                    selectedPerson?.id === person.id ? 'bg-primary-50' : ''
                  }`}
                >
                  <div className="font-medium truncate">{person.name}</div>
                  <div className="text-gray-500 capitalize">{person.type}</div>
                </button>
              ))}
            </div>
            
            {/* Biography editor */}
            <div className="col-span-2 p-3 overflow-y-auto">
              {selectedPerson ? (
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <div>
                      <div className="font-semibold">{selectedPerson.name}</div>
                      <div className="text-xs text-gray-500 capitalize">{selectedPerson.role}</div>
                    </div>
                    {bioEditing ? (
                      <button
                        onClick={handleSaveBiography}
                        disabled={actionLoading}
                        className="flex items-center gap-1 px-3 py-1 bg-green-500 text-white text-xs rounded hover:bg-green-600"
                      >
                        {actionLoading ? <Loader2 className="w-3 h-3 animate-spin" /> : <Save className="w-3 h-3" />}
                        Save
                      </button>
                    ) : (
                      <button
                        onClick={() => setBioEditing(true)}
                        className="flex items-center gap-1 px-3 py-1 bg-primary-500 text-white text-xs rounded hover:bg-primary-600"
                      >
                        <Edit3 className="w-3 h-3" /> Edit
                      </button>
                    )}
                  </div>
                  
                  {bioEditing ? (
                    <div className="space-y-2">
                      <div>
                        <label className="text-xs font-medium">Bio</label>
                        <textarea
                          value={biography?.bio || ''}
                          onChange={e => setBiography(prev => prev ? {...prev, bio: e.target.value} : null)}
                          className="w-full p-2 border rounded text-xs h-16"
                          placeholder="Short description..."
                        />
                      </div>
                      <div>
                        <label className="text-xs font-medium">Background</label>
                        <textarea
                          value={biography?.background || ''}
                          onChange={e => setBiography(prev => prev ? {...prev, background: e.target.value} : null)}
                          className="w-full p-2 border rounded text-xs h-16"
                          placeholder="Experience, education..."
                        />
                      </div>
                      <div>
                        <label className="text-xs font-medium">Personality</label>
                        <textarea
                          value={biography?.personality || ''}
                          onChange={e => setBiography(prev => prev ? {...prev, personality: e.target.value} : null)}
                          className="w-full p-2 border rounded text-xs h-16"
                          placeholder="How they think and act..."
                        />
                      </div>
                    </div>
                  ) : (
                    <div className="space-y-2 text-xs">
                      {biography?.bio && (
                        <div><span className="font-medium">Bio:</span> {biography.bio}</div>
                      )}
                      {biography?.background && (
                        <div><span className="font-medium">Background:</span> {biography.background}</div>
                      )}
                      {biography?.personality && (
                        <div><span className="font-medium">Personality:</span> {biography.personality}</div>
                      )}
                      {!biography?.bio && !biography?.background && !biography?.personality && (
                        <div className="text-gray-400 italic">No biography set. Click Edit to add one.</div>
                      )}
                    </div>
                  )}
                </div>
              ) : (
                <div className="flex items-center justify-center h-full text-gray-400 text-sm">
                  Select a person to view/edit their biography
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Activity Log and Working Employees - Full Width */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* Live Activity Log */}
        <div className="bg-white rounded-lg shadow">
          <div className="px-4 py-3 border-b flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Activity className="w-5 h-5 text-green-600" />
              <h2 className="font-semibold">Live Activity Log</h2>
            </div>
            <span className="text-xs text-gray-400">{activityLog.length} events</span>
          </div>
          <div className="divide-y max-h-80 overflow-y-auto font-mono text-xs">
            {activityLog.length === 0 ? (
              <div className="p-4 text-center text-gray-500">Waiting for activity...</div>
            ) : (
              activityLog.slice(0, 50).map(entry => (
                <div key={entry.id} className={`px-3 py-2 ${
                  entry.type === 'work_complete' && entry.metadata?.has_error ? 'bg-red-50' :
                  entry.type === 'work_complete' ? 'bg-green-50' :
                  entry.type === 'work_started' ? 'bg-blue-50' :
                  entry.type === 'system' ? 'bg-gray-50' : ''
                }`}>
                  <div className="flex items-start gap-2">
                    <span className="text-gray-400 whitespace-nowrap">
                      {new Date(entry.timestamp).toLocaleTimeString()}
                    </span>
                    <span className={`px-1 rounded text-[10px] uppercase ${
                      entry.type === 'work_complete' ? 'bg-green-200 text-green-800' :
                      entry.type === 'work_started' ? 'bg-blue-200 text-blue-800' :
                      entry.type === 'employee_update' ? 'bg-purple-200 text-purple-800' :
                      'bg-gray-200 text-gray-600'
                    }`}>
                      {entry.type.replace('_', ' ')}
                    </span>
                    <span className="flex-1 truncate">{entry.title}</span>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>

        {/* Working Employees */}
        <div className="bg-white rounded-lg shadow">
          <div className="px-4 py-3 border-b flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Users className="w-5 h-5 text-blue-600" />
              <h2 className="font-semibold">Working Employees</h2>
            </div>
            <span className="text-xs px-2 py-1 bg-blue-100 text-blue-800 rounded-full">
              {orgStats?.by_status?.working || employees.filter(e => e.status === 'working').length} active
            </span>
          </div>
          <div className="divide-y max-h-80 overflow-y-auto">
            {employees.filter(e => e.status === 'working').length === 0 ? (
              <div className="p-4 text-center text-gray-500">No employees currently working</div>
            ) : (
              employees.filter(e => e.status === 'working').map(emp => (
                <div key={emp.id} className="px-4 py-3">
                  <div className="flex items-center justify-between">
                    <div>
                      <div className="font-medium text-sm">{emp.name}</div>
                      <div className="text-xs text-gray-500 capitalize">{emp.skill}</div>
                    </div>
                    <div className="text-right">
                      <div className="text-xs font-medium text-blue-600">{emp.current_work || 'Processing...'}</div>
                      <div className="text-[10px] text-gray-400">{emp.work_count || 0} tasks completed</div>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      </div>

      {/* Component health */}
      {health?.checks && (
        <div className="bg-white rounded-lg shadow">
          <div className="px-4 py-3 border-b flex items-center gap-2">
            <RefreshCw className="w-5 h-5 text-gray-600" />
            <h2 className="font-semibold">System Health</h2>
          </div>
          <div className="p-4 grid grid-cols-2 md:grid-cols-4 gap-3">
            {Object.entries(health.checks).map(([name, check]) => (
              <div key={name} className="flex items-center justify-between p-2 bg-gray-50 rounded text-sm">
                <span className="font-medium capitalize">{name}</span>
                <span className={`flex items-center gap-1 ${check.status === 'up' ? 'text-green-600' : 'text-red-600'}`}>
                  {check.status === 'up' ? <CheckCircle className="w-4 h-4" /> : <XCircle className="w-4 h-4" />}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

// Workflows page
function Workflows() {
  const [workflows, setWorkflows] = useState<WorkflowTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [starting, setStarting] = useState<string | null>(null)
  const [inputs, setInputs] = useState<Record<string, string>>({})

  useEffect(() => {
    api.getWorkflows()
      .then(data => setWorkflows(data.workflows || []))
      .finally(() => setLoading(false))
  }, [])

  const handleStart = async (templateId: string) => {
    setStarting(templateId)
    try {
      await api.startRun(templateId, inputs)
      setInputs({})
    } catch (err) {
      console.error('Failed to start workflow:', err)
    } finally {
      setStarting(null)
    }
  }

  if (loading) {
    return <div className="flex items-center justify-center h-64"><Loader2 className="w-8 h-8 animate-spin text-primary-500" /></div>
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Workflows</h1>
      
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {workflows.length === 0 ? (
          <div className="col-span-2 bg-white rounded-lg shadow p-8 text-center text-gray-500">
            No workflow templates found. Add YAML files to the workflows directory.
          </div>
        ) : (
          workflows.map(wf => (
            <div key={wf.id} className="bg-white rounded-lg shadow p-4">
              <div className="flex justify-between items-start mb-2">
                <div>
                  <h3 className="font-semibold text-lg">{wf.name}</h3>
                  <p className="text-sm text-gray-500">{wf.description}</p>
                </div>
                <span className="text-xs text-gray-400">v{wf.version}</span>
              </div>
              
              <div className="mt-4 space-y-2">
                <input
                  type="text"
                  placeholder="Enter domain (e.g., fintech)"
                  className="w-full px-3 py-2 border rounded text-sm"
                  value={inputs[wf.id] || ''}
                  onChange={(e) => setInputs({ ...inputs, [wf.id]: e.target.value })}
                />
                <button
                  onClick={() => handleStart(wf.id)}
                  disabled={starting === wf.id}
                  className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-primary-600 text-white rounded hover:bg-primary-700 disabled:opacity-50"
                >
                  {starting === wf.id ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
                  Start Workflow
                </button>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  )
}

// Run detail page
function RunDetail() {
  const location = useLocation()
  const runId = location.pathname.split('/').pop()
  const [run, setRun] = useState<WorkflowRun | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!runId) return
    const fetchRun = async () => {
      try {
        const data = await api.getRun(runId)
        setRun(data)
      } finally {
        setLoading(false)
      }
    }
    fetchRun()
    const interval = setInterval(fetchRun, 3000)
    return () => clearInterval(interval)
  }, [runId])

  if (loading) {
    return <div className="flex items-center justify-center h-64"><Loader2 className="w-8 h-8 animate-spin text-primary-500" /></div>
  }

  if (!run) {
    return <div className="text-center text-gray-500 py-8">Run not found</div>
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <Link to="/runs" className="text-primary-600 hover:underline text-sm">← Back to Runs</Link>
          <h1 className="text-2xl font-bold mt-1">{run.template_id}</h1>
        </div>
        <StatusBadge status={run.status} />
      </div>

      <div className="bg-white rounded-lg shadow p-4">
        <h2 className="font-semibold mb-2">Details</h2>
        <dl className="grid grid-cols-2 gap-2 text-sm">
          <dt className="text-gray-500">Run ID</dt>
          <dd className="font-mono text-xs">{run.id}</dd>
          <dt className="text-gray-500">Started</dt>
          <dd>{run.started_at ? new Date(run.started_at).toLocaleString() : '-'}</dd>
          <dt className="text-gray-500">Completed</dt>
          <dd>{run.completed_at ? new Date(run.completed_at).toLocaleString() : '-'}</dd>
          {run.error && (
            <>
              <dt className="text-gray-500">Error</dt>
              <dd className="text-red-600">{run.error}</dd>
            </>
          )}
        </dl>
      </div>

      {run.steps && run.steps.length > 0 && (
        <div className="bg-white rounded-lg shadow">
          <div className="px-4 py-3 border-b">
            <h2 className="font-semibold">Steps</h2>
          </div>
          <div className="divide-y">
            {run.steps.map(step => (
              <div key={step.id} className="p-4">
                <div className="flex items-center justify-between mb-2">
                  <div className="flex items-center gap-2">
                    <span className="font-medium">{step.step_id}</span>
                    <span className="text-xs text-gray-500 capitalize">({step.role})</span>
                  </div>
                  <div className="flex items-center gap-2">
                    {step.latency_ms > 0 && <span className="text-xs text-gray-400">{step.latency_ms}ms</span>}
                    {step.tokens_used > 0 && <span className="text-xs text-gray-400">{step.tokens_used} tokens</span>}
                    <StatusBadge status={step.status} />
                  </div>
                </div>
                {step.response && (
                  <pre className="mt-2 p-2 bg-gray-50 rounded text-xs overflow-x-auto whitespace-pre-wrap">
                    {step.response.slice(0, 500)}{step.response.length > 500 ? '...' : ''}
                  </pre>
                )}
                {step.error && (
                  <div className="mt-2 text-red-600 text-sm">{step.error}</div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

// Runs list page
function Runs() {
  const [runs, setRuns] = useState<WorkflowRun[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('')

  useEffect(() => {
    api.getRuns(filter)
      .then(data => setRuns(data.runs || []))
      .finally(() => setLoading(false))
  }, [filter])

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Workflow Runs</h1>
        <select
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="px-3 py-2 border rounded"
        >
          <option value="">All</option>
          <option value="running">Running</option>
          <option value="completed">Completed</option>
          <option value="failed">Failed</option>
          <option value="cancelled">Cancelled</option>
        </select>
      </div>

      {loading ? (
        <div className="flex items-center justify-center h-64"><Loader2 className="w-8 h-8 animate-spin text-primary-500" /></div>
      ) : (
        <div className="bg-white rounded-lg shadow divide-y">
          {runs.length === 0 ? (
            <div className="p-8 text-center text-gray-500">No runs found</div>
          ) : (
            runs.map(run => (
              <Link key={run.id} to={`/runs/${run.id}`} className="block p-4 hover:bg-gray-50">
                <div className="flex items-center justify-between">
                  <div>
                    <div className="font-medium">{run.template_id}</div>
                    <div className="text-sm text-gray-500 font-mono">{run.id.slice(0, 8)}</div>
                  </div>
                  <div className="text-right">
                    <StatusBadge status={run.status} />
                    <div className="text-xs text-gray-400 mt-1">
                      {new Date(run.created_at).toLocaleString()}
                    </div>
                  </div>
                </div>
              </Link>
            ))
          )}
        </div>
      )}
    </div>
  )
}

// Organization page
function OrganizationPage() {
  const [divisions, setDivisions] = useState<Division[]>([])
  const [employees, setEmployees] = useState<Employee[]>([])
  const [companyStatus, setCompanyStatus] = useState<CompanyStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [selectedPerson, setSelectedPerson] = useState<PersonDetail | null>(null)
  const [loadingPerson, setLoadingPerson] = useState(false)

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [divisionsData, employeesData, statusData] = await Promise.all([
          api.getDivisions(),
          api.getEmployees(),
          api.getOrgStatus()
        ])
        setDivisions(divisionsData.divisions || [])
        setEmployees(employeesData.employees || [])
        setCompanyStatus(statusData)
      } finally {
        setLoading(false)
      }
    }
    fetchData()
    const interval = setInterval(fetchData, 5000)
    return () => clearInterval(interval)
  }, [])

  const handleSelectPerson = async (id: string) => {
    setLoadingPerson(true)
    try {
      const detail = await api.getPersonDetail(id)
      setSelectedPerson(detail)
    } finally {
      setLoadingPerson(false)
    }
  }

  if (loading) {
    return <div className="flex items-center justify-center h-64"><Loader2 className="w-8 h-8 animate-spin text-primary-500" /></div>
  }

  return (
    <div className="flex gap-6">
      {/* Left column - Org structure */}
      <div className="flex-1 space-y-6">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold">Organization Structure</h1>
          <div className="flex items-center gap-2">
            <span className={`w-3 h-3 rounded-full ${companyStatus?.status === 'running' ? 'bg-green-500' : 'bg-yellow-500'}`} />
            <span className="text-sm capitalize">{companyStatus?.status}</span>
          </div>
        </div>

        {/* Divisions */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {divisions.map((div: Division) => (
            <div key={div.id} className="bg-white rounded-lg shadow p-4">
              <div className="flex items-center gap-2 mb-2">
                <Building2 className="w-5 h-5 text-purple-600" />
                <h3 className="font-semibold">{div.name}</h3>
              </div>
              <p className="text-sm text-gray-500 mb-2">{div.description}</p>
              <div className="text-xs text-primary-600">{div.departments} departments</div>
            </div>
          ))}
        </div>

        {/* Employee Stats */}
        <div className="bg-white rounded-lg shadow p-4">
          <h2 className="text-lg font-semibold mb-4">Employee Distribution</h2>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
            {Object.entries(companyStatus?.stats?.by_skill || {}).map(([skill, count]) => (
              <div key={skill} className="bg-gray-50 rounded p-3 text-center">
                <div className="text-xl font-bold text-primary-600">{count as number}</div>
                <div className="text-xs text-gray-500 capitalize">{skill.replace('_', ' ')}</div>
              </div>
            ))}
          </div>
        </div>

        {/* Employee List */}
        <div className="bg-white rounded-lg shadow">
          <div className="px-4 py-3 border-b">
            <h2 className="font-semibold">All Employees ({employees.length})</h2>
          </div>
          <div className="divide-y max-h-80 overflow-y-auto">
            {employees.map((emp: Employee) => (
              <div 
                key={emp.id} 
                className={`p-3 flex items-center justify-between cursor-pointer hover:bg-gray-50 ${selectedPerson?.id === emp.id ? 'bg-primary-50' : ''}`}
                onClick={() => handleSelectPerson(emp.id)}
              >
                <div className="flex items-center gap-3">
                  <UserCircle className="w-8 h-8 text-gray-400" />
                  <div>
                    <div className="font-medium">{emp.name}</div>
                    <div className="text-xs text-gray-500 capitalize">{emp.skill.replace('_', ' ')}</div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-xs text-gray-400">{emp.work_count} tasks</span>
                  <StatusBadge status={emp.status} />
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Right column - Person Detail Card */}
      <div className="w-96">
        <div className="bg-white rounded-lg shadow sticky top-6">
          {loadingPerson ? (
            <div className="p-8 text-center">
              <Loader2 className="w-8 h-8 animate-spin text-primary-500 mx-auto" />
            </div>
          ) : selectedPerson ? (
            <div className="p-4 space-y-4">
              {/* Header */}
              <div className="text-center border-b pb-4">
                <UserCircle className="w-16 h-16 mx-auto text-primary-500 mb-2" />
                <h2 className="text-xl font-bold">{selectedPerson.name}</h2>
                <p className="text-sm text-gray-500">{selectedPerson.title}</p>
                <span className="inline-block mt-2 px-2 py-1 text-xs bg-primary-100 text-primary-700 rounded capitalize">
                  {selectedPerson.type.replace('_', ' ')}
                </span>
              </div>

              {/* Boss/Manager */}
              <div>
                <h3 className="text-sm font-semibold text-gray-700 mb-2">Reports To</h3>
                {selectedPerson.boss ? (
                  <div 
                    className="flex items-center gap-2 p-2 bg-gray-50 rounded cursor-pointer hover:bg-gray-100"
                    onClick={() => handleSelectPerson(selectedPerson.boss!.id)}
                  >
                    <UserCircle className="w-6 h-6 text-gray-400" />
                    <div>
                      <div className="text-sm font-medium">{selectedPerson.boss.name}</div>
                      <div className="text-xs text-gray-500">{selectedPerson.boss.title}</div>
                    </div>
                  </div>
                ) : (
                  <p className="text-sm text-gray-400 italic">Top of organization</p>
                )}
              </div>

              {/* Direct Reports */}
              <div>
                <h3 className="text-sm font-semibold text-gray-700 mb-2">
                  Direct Reports ({selectedPerson.direct_reports?.length || 0})
                </h3>
                {selectedPerson.direct_reports && selectedPerson.direct_reports.length > 0 ? (
                  <div className="space-y-1 max-h-32 overflow-y-auto">
                    {selectedPerson.direct_reports.map((report: { id: string; name: string; title: string }) => (
                      <div 
                        key={report.id} 
                        className="flex items-center gap-2 p-2 bg-gray-50 rounded cursor-pointer hover:bg-gray-100"
                        onClick={() => handleSelectPerson(report.id)}
                      >
                        <UserCircle className="w-5 h-5 text-gray-400" />
                        <div>
                          <div className="text-sm font-medium">{report.name}</div>
                          <div className="text-xs text-gray-500">{report.title}</div>
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-sm text-gray-400 italic">No direct reports</p>
                )}
              </div>

              {/* Expectations */}
              <div>
                <h3 className="text-sm font-semibold text-gray-700 mb-2">Expectations</h3>
                {selectedPerson.expectations && selectedPerson.expectations.length > 0 ? (
                  <ul className="space-y-1">
                    {selectedPerson.expectations.map((exp: string, i: number) => (
                      <li key={i} className="flex items-start gap-2 text-sm text-gray-600">
                        <CheckCircle className="w-4 h-4 text-green-500 mt-0.5 flex-shrink-0" />
                        <span>{exp}</span>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-sm text-gray-400 italic">No expectations defined</p>
                )}
              </div>

              {/* Additional info */}
              {selectedPerson.status && (
                <div className="pt-2 border-t">
                  <div className="flex justify-between text-sm">
                    <span className="text-gray-500">Status</span>
                    <StatusBadge status={selectedPerson.status} />
                  </div>
                </div>
              )}

              {/* View Full Details link */}
              {selectedPerson.type === 'employee' && (
                <div className="pt-3 border-t">
                  <Link 
                    to={`/org/employee/${selectedPerson.id}`}
                    className="block w-full text-center px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 text-sm font-medium"
                  >
                    View Full Details →
                  </Link>
                </div>
              )}
            </div>
          ) : (
            <div className="p-8 text-center text-gray-400">
              <UserCircle className="w-16 h-16 mx-auto mb-2 opacity-50" />
              <p>Select a person to view details</p>
              <p className="text-xs mt-2">
                Click on any employee in the list to see their:<br/>
                name, title, boss, reports & expectations
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

// People page with biography editing
function PeoplePage() {
  const [people, setPeople] = useState<Person[]>([])
  const [selectedPerson, setSelectedPerson] = useState<Person | null>(null)
  const [biography, setBiography] = useState<Biography | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    api.getPeople().then(data => {
      setPeople(data.people || [])
      setLoading(false)
    })
  }, [])

  const handleSelect = async (person: Person) => {
    setSelectedPerson(person)
    try {
      const bio = await api.getBiography(person.id)
      setBiography(bio)
    } catch {
      setBiography({
        person_id: person.id,
        person_type: person.type,
        name: person.name,
        bio: '',
        background: '',
        personality: '',
        goals: [],
        values: [],
        quirks: []
      })
    }
  }

  const handleSave = async () => {
    if (!biography) return
    setSaving(true)
    try {
      await api.updateBiography(biography)
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return <div className="flex items-center justify-center h-64"><Loader2 className="w-8 h-8 animate-spin text-primary-500" /></div>
  }

  return (
    <div className="grid grid-cols-3 gap-6 h-[calc(100vh-8rem)]">
      {/* People list */}
      <div className="bg-white rounded-lg shadow overflow-hidden">
        <div className="px-4 py-3 border-b bg-gray-50">
          <h2 className="font-semibold">All People ({people.length})</h2>
        </div>
        <div className="divide-y overflow-y-auto h-[calc(100%-3rem)]">
          {people.map(person => (
            <button
              key={person.id}
              onClick={() => handleSelect(person)}
              className={`w-full text-left p-3 hover:bg-gray-50 ${selectedPerson?.id === person.id ? 'bg-primary-50 border-l-4 border-primary-500' : ''}`}
            >
              <div className="flex items-center gap-2">
                <UserCircle className="w-8 h-8 text-gray-400" />
                <div>
                  <div className="font-medium">{person.name}</div>
                  <div className="text-xs text-gray-500 capitalize">{person.type} - {person.role}</div>
                </div>
              </div>
            </button>
          ))}
        </div>
      </div>

      {/* Biography editor */}
      <div className="col-span-2 bg-white rounded-lg shadow overflow-hidden">
        {selectedPerson ? (
          <div className="h-full flex flex-col">
            <div className="px-4 py-3 border-b bg-gray-50 flex items-center justify-between">
              <div>
                <h2 className="font-semibold">{selectedPerson.name}</h2>
                <p className="text-xs text-gray-500 capitalize">{selectedPerson.type} - {selectedPerson.role}</p>
              </div>
              <button
                onClick={handleSave}
                disabled={saving}
                className="flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded hover:bg-primary-700 disabled:opacity-50"
              >
                {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
                Save Biography
              </button>
            </div>
            <div className="flex-1 overflow-y-auto p-4 space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Bio</label>
                <textarea
                  value={biography?.bio || ''}
                  onChange={e => setBiography(prev => prev ? {...prev, bio: e.target.value} : null)}
                  className="w-full p-3 border rounded-lg h-24"
                  placeholder="A short description of who they are..."
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Background</label>
                <textarea
                  value={biography?.background || ''}
                  onChange={e => setBiography(prev => prev ? {...prev, background: e.target.value} : null)}
                  className="w-full p-3 border rounded-lg h-24"
                  placeholder="Their history, experience, education..."
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Personality</label>
                <textarea
                  value={biography?.personality || ''}
                  onChange={e => setBiography(prev => prev ? {...prev, personality: e.target.value} : null)}
                  className="w-full p-3 border rounded-lg h-24"
                  placeholder="How they think, communicate, and make decisions..."
                />
              </div>
              <div className="bg-gray-50 rounded-lg p-4">
                <h3 className="text-sm font-medium text-gray-700 mb-2">How Biography Affects Behavior</h3>
                <p className="text-xs text-gray-500">
                  The biography you enter is used to create an LLM persona for this person. 
                  When they perform work or make decisions, their personality, background, and values 
                  will influence their output. Changes take effect immediately.
                </p>
              </div>
            </div>
          </div>
        ) : (
          <div className="flex items-center justify-center h-full text-gray-400">
            <div className="text-center">
              <UserCircle className="w-16 h-16 mx-auto mb-2 opacity-50" />
              <p>Select a person to view and edit their biography</p>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// Seed Setup page
function SeedSetupPage() {
  const [sectors, setSectors] = useState<SectorInfo[]>([])
  const [seed, setSeed] = useState<CompanySeed | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [form, setForm] = useState({
    sector: '',
    custom_sector: '',
    company_name: '',
    target_market: '',
    mission: '',
    vision: ''
  })

  useEffect(() => {
    Promise.all([api.getSectors(), api.getSeed()]).then(([sectorsData, seedData]) => {
      setSectors(sectorsData.sectors || [])
      if (seedData.seeded && seedData.seed) {
        setSeed(seedData.seed)
        setForm({
          sector: seedData.seed.sector,
          custom_sector: seedData.seed.custom_sector || '',
          company_name: seedData.seed.company_name,
          target_market: seedData.seed.target_market,
          mission: seedData.seed.mission,
          vision: seedData.seed.vision
        })
      }
      setLoading(false)
    })
  }, [])

  const handleSubmit = async () => {
    if (!form.sector || !form.company_name) return
    setSaving(true)
    try {
      const result = await api.setSeed(form)
      setSeed(result.seed)
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return <div className="flex items-center justify-center h-64"><Loader2 className="w-8 h-8 animate-spin text-primary-500" /></div>
  }

  return (
    <div className="max-w-4xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <Rocket className="w-8 h-8 text-primary-600" />
        <div>
          <h1 className="text-2xl font-bold">Company Seed</h1>
          <p className="text-gray-500">Bootstrap the AI Corporation with a business sector</p>
        </div>
      </div>

      {seed && seed.active && (
        <div className="bg-green-50 border border-green-200 rounded-lg p-4">
          <div className="flex items-center gap-2 text-green-800 font-medium mb-2">
            <CheckCircle className="w-5 h-5" />
            Company Seeded
          </div>
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div><span className="text-gray-500">Company:</span> {seed.company_name}</div>
            <div><span className="text-gray-500">Sector:</span> {seed.sector === 'custom' ? seed.custom_sector : seed.sector}</div>
            <div><span className="text-gray-500">Target:</span> {seed.target_market}</div>
            <div><span className="text-gray-500">Since:</span> {seed.created_at ? new Date(seed.created_at).toLocaleDateString() : 'N/A'}</div>
          </div>
          <div className="mt-3 pt-3 border-t border-green-200">
            <div className="text-sm"><span className="text-gray-500">Mission:</span> {seed.mission}</div>
            <div className="text-sm mt-1"><span className="text-gray-500">Vision:</span> {seed.vision}</div>
          </div>
        </div>
      )}

      <div className="bg-white rounded-lg shadow p-6 space-y-6">
        <div className="flex items-center gap-2 text-lg font-semibold">
          <Target className="w-5 h-5 text-primary-600" />
          {seed ? 'Update Business Configuration' : 'Select Business Sector'}
        </div>

        {/* Sector Grid */}
        <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
          {sectors.map(sector => (
            <button
              key={sector.id}
              onClick={() => setForm(f => ({ ...f, sector: sector.id }))}
              className={`text-left p-3 rounded-lg border-2 transition-colors ${
                form.sector === sector.id
                  ? 'border-primary-500 bg-primary-50'
                  : 'border-gray-200 hover:border-gray-300'
              }`}
            >
              <div className="font-medium text-sm">{sector.name}</div>
              <div className="text-xs text-gray-500 mt-1">{sector.description}</div>
            </button>
          ))}
        </div>

        {form.sector === 'custom' && (
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Custom Sector</label>
            <input
              type="text"
              value={form.custom_sector}
              onChange={e => setForm(f => ({ ...f, custom_sector: e.target.value }))}
              className="w-full p-3 border rounded-lg"
              placeholder="Describe your business sector..."
            />
          </div>
        )}

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Company Name *</label>
            <input
              type="text"
              value={form.company_name}
              onChange={e => setForm(f => ({ ...f, company_name: e.target.value }))}
              className="w-full p-3 border rounded-lg"
              placeholder="AI Ventures Inc."
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Target Market</label>
            <input
              type="text"
              value={form.target_market}
              onChange={e => setForm(f => ({ ...f, target_market: e.target.value }))}
              className="w-full p-3 border rounded-lg"
              placeholder="e.g., Young professionals, Enterprise, SMBs"
            />
          </div>
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Mission Statement (optional, auto-generated if blank)</label>
          <textarea
            value={form.mission}
            onChange={e => setForm(f => ({ ...f, mission: e.target.value }))}
            className="w-full p-3 border rounded-lg h-20"
            placeholder="Leave blank to auto-generate with AI..."
          />
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Vision Statement (optional, auto-generated if blank)</label>
          <textarea
            value={form.vision}
            onChange={e => setForm(f => ({ ...f, vision: e.target.value }))}
            className="w-full p-3 border rounded-lg h-20"
            placeholder="Leave blank to auto-generate with AI..."
          />
        </div>

        <div className="flex justify-end">
          <button
            onClick={handleSubmit}
            disabled={saving || !form.sector || !form.company_name}
            className="flex items-center gap-2 px-6 py-3 bg-primary-600 text-white rounded-lg font-medium hover:bg-primary-700 disabled:opacity-50"
          >
            {saving ? <Loader2 className="w-5 h-5 animate-spin" /> : <Rocket className="w-5 h-5" />}
            {seed ? 'Update Seed' : 'Launch Company'}
          </button>
        </div>
      </div>

      <div className="bg-gray-50 rounded-lg p-4 text-sm text-gray-600">
        <h3 className="font-medium text-gray-800 mb-2">How the seed works</h3>
        <ul className="space-y-1 list-disc list-inside">
          <li>The business sector determines the types of projects and ideas the AI will generate</li>
          <li>All employees and board members will make decisions aligned with this sector</li>
          <li>Mission and vision statements are injected into every AI persona</li>
          <li>You can update the seed at any time to pivot the company's focus</li>
        </ul>
      </div>
    </div>
  )
}

// Employee Detail Page
function EmployeeDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [employee, setEmployee] = useState<EmployeeDetail | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!id) return
    const fetchEmployee = async () => {
      try {
        const data = await api.getEmployeeDetail(id)
        setEmployee(data)
      } finally {
        setLoading(false)
      }
    }
    fetchEmployee()
    const interval = setInterval(fetchEmployee, 5000)
    return () => clearInterval(interval)
  }, [id])

  if (loading) {
    return <div className="flex items-center justify-center h-64"><Loader2 className="w-8 h-8 animate-spin text-primary-500" /></div>
  }

  if (!employee) {
    return <div className="text-center text-gray-500 py-8">Employee not found</div>
  }

  const activityTypeColors: Record<string, string> = {
    assigned: 'bg-blue-500',
    started: 'bg-green-500',
    completed: 'bg-green-600',
    paused: 'bg-yellow-500',
    resumed: 'bg-blue-400',
    decision: 'bg-purple-500',
    action: 'bg-indigo-500',
    result: 'bg-teal-500',
    review: 'bg-orange-500',
    revision: 'bg-amber-500',
    error: 'bg-red-500',
  }

  return (
    <div className="space-y-6">
      {/* Header with back button */}
      <div className="flex items-center gap-4">
        <button 
          onClick={() => navigate('/org')}
          className="p-2 hover:bg-gray-100 rounded"
        >
          <ChevronLeft className="w-5 h-5" />
        </button>
        <div className="flex-1">
          <h1 className="text-2xl font-bold">{employee.name}</h1>
          <p className="text-sm text-gray-500 capitalize">{employee.skill.replace('_', ' ')}</p>
        </div>
        <StatusBadge status={employee.status} />
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div className="bg-white rounded-lg shadow p-4 text-center">
          <div className="text-2xl font-bold text-primary-600">{employee.stats.completed_tasks}</div>
          <div className="text-xs text-gray-500">Completed Tasks</div>
        </div>
        <div className="bg-white rounded-lg shadow p-4 text-center">
          <div className="text-2xl font-bold text-blue-600">{employee.stats.total_work_time}</div>
          <div className="text-xs text-gray-500">Total Work Time</div>
        </div>
        <div className="bg-white rounded-lg shadow p-4 text-center">
          <div className="text-2xl font-bold text-green-600">{employee.stats.average_task_time}</div>
          <div className="text-xs text-gray-500">Avg Task Time</div>
        </div>
        <div className="bg-white rounded-lg shadow p-4 text-center">
          <div className="text-2xl font-bold text-purple-600">{employee.stats.activity_count}</div>
          <div className="text-xs text-gray-500">Activity Count</div>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Current Work */}
        <div className="bg-white rounded-lg shadow p-4">
          <h2 className="text-lg font-semibold mb-4">Current Work</h2>
          {employee.current_work ? (
            <div className="space-y-3">
              <div>
                <div className="font-medium">{employee.current_work.title}</div>
                <div className="text-sm text-gray-500">{employee.current_work.description}</div>
              </div>
              <div className="flex items-center gap-4 text-sm">
                <span className="text-gray-500">Type: <span className="text-gray-700">{employee.current_work.type}</span></span>
                <span className="text-gray-500">Priority: <span className="text-gray-700">{employee.current_work.priority}</span></span>
              </div>
              {employee.time_on_task && (
                <div className="flex items-center gap-2 text-sm">
                  <Clock className="w-4 h-4 text-gray-400" />
                  <span className="text-gray-600">Working for: <span className="font-medium text-primary-600">{employee.time_on_task}</span></span>
                </div>
              )}
              {employee.current_work.objectives && employee.current_work.objectives.length > 0 && (
                <div className="mt-2">
                  <div className="text-sm font-medium text-gray-700 mb-1">Objectives:</div>
                  <ul className="text-sm text-gray-600 space-y-1">
                    {employee.current_work.objectives.map((obj: string, i: number) => (
                      <li key={i} className="flex items-start gap-2">
                        <CheckCircle className="w-4 h-4 text-green-500 mt-0.5 flex-shrink-0" />
                        {obj}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          ) : (
            <div className="text-center text-gray-400 py-8">
              <UserCircle className="w-12 h-12 mx-auto mb-2 opacity-50" />
              <p>Currently idle</p>
            </div>
          )}
        </div>

        {/* Employee Info */}
        <div className="bg-white rounded-lg shadow p-4">
          <h2 className="text-lg font-semibold mb-4">Employee Info</h2>
          <div className="space-y-3 text-sm">
            <div className="flex justify-between">
              <span className="text-gray-500">Hired</span>
              <span>{new Date(employee.hired_at).toLocaleDateString()}</span>
            </div>
            {employee.manager && (
              <div className="flex justify-between">
                <span className="text-gray-500">Manager</span>
                <span 
                  className="text-primary-600 cursor-pointer hover:underline"
                  onClick={() => navigate(`/org/employee/${employee.manager!.id}`)}
                >
                  {employee.manager.name}
                </span>
              </div>
            )}
            <div className="flex justify-between">
              <span className="text-gray-500">Tasks Completed</span>
              <span>{employee.work_count}</span>
            </div>
            {employee.persona && (
              <div className="pt-2 border-t">
                <div className="text-gray-500 mb-1">Persona</div>
                <div className="text-gray-700 text-xs bg-gray-50 p-2 rounded max-h-32 overflow-y-auto">
                  {employee.persona}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Activity Log */}
      <div className="bg-white rounded-lg shadow">
        <div className="px-4 py-3 border-b">
          <h2 className="font-semibold">Activity Log ({employee.activity_log?.length || 0})</h2>
        </div>
        <div className="divide-y max-h-96 overflow-y-auto">
          {employee.activity_log && employee.activity_log.length > 0 ? (
            [...employee.activity_log].reverse().map((entry: ActivityLogEntry) => (
              <div key={entry.id} className="p-3 flex gap-3">
                <div className={`w-2 h-2 rounded-full mt-2 flex-shrink-0 ${activityTypeColors[entry.type] || 'bg-gray-400'}`} />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-sm">{entry.title}</span>
                    <span className="text-xs px-1.5 py-0.5 bg-gray-100 rounded capitalize">{entry.type}</span>
                  </div>
                  <p className="text-sm text-gray-600 mt-0.5">{entry.description}</p>
                  <div className="text-xs text-gray-400 mt-1">
                    {new Date(entry.timestamp).toLocaleString()}
                  </div>
                </div>
              </div>
            ))
          ) : (
            <div className="p-8 text-center text-gray-400">No activity recorded yet</div>
          )}
        </div>
      </div>

      {/* Work History */}
      <div className="bg-white rounded-lg shadow">
        <div className="px-4 py-3 border-b">
          <h2 className="font-semibold">Work History ({employee.work_history?.length || 0})</h2>
        </div>
        <div className="divide-y max-h-96 overflow-y-auto">
          {employee.work_history && employee.work_history.length > 0 ? (
            [...employee.work_history].reverse().map((result: WorkResult) => (
              <div key={result.id} className="p-3">
                <div className="flex items-center justify-between mb-1">
                  <span className="text-sm font-medium">Task Completed</span>
                  <span className="text-xs text-gray-400">{new Date(result.completed_at).toLocaleString()}</span>
                </div>
                <p className="text-sm text-gray-600 line-clamp-2">{result.output}</p>
                <div className="flex items-center gap-4 mt-2 text-xs text-gray-500">
                  <span>Tokens: {result.tokens_used}</span>
                  <span>Duration: {Math.round(result.duration / 1000000000)}s</span>
                </div>
              </div>
            ))
          ) : (
            <div className="p-8 text-center text-gray-400">No work history yet</div>
          )}
        </div>
      </div>
    </div>
  )
}

// Meetings List Page
function MeetingsPage() {
  const [meetings, setMeetings] = useState<MeetingSummary[]>([])
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()

  useEffect(() => {
    const fetchMeetings = async () => {
      try {
        const data = await api.listMeetings()
        setMeetings(data.meetings || [])
      } finally {
        setLoading(false)
      }
    }
    fetchMeetings()
  }, [])

  const statusColors: Record<string, string> = {
    scheduled: 'bg-blue-100 text-blue-700',
    in_progress: 'bg-yellow-100 text-yellow-700',
    completed: 'bg-green-100 text-green-700',
    cancelled: 'bg-gray-100 text-gray-500',
  }

  const typeColors: Record<string, string> = {
    regular: 'bg-gray-100 text-gray-600',
    emergency: 'bg-red-100 text-red-700',
    quarterly: 'bg-purple-100 text-purple-700',
  }

  if (loading) {
    return <div className="flex items-center justify-center h-64"><Loader2 className="w-8 h-8 animate-spin text-primary-500" /></div>
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Meeting Notes</h1>
        <span className="text-sm text-gray-500">{meetings.length} meetings</span>
      </div>

      {meetings.length === 0 ? (
        <div className="bg-white rounded-lg shadow p-8 text-center text-gray-400">
          <Activity className="w-12 h-12 mx-auto mb-4 opacity-50" />
          <p>No meetings recorded yet</p>
          <p className="text-xs mt-2">Meetings will appear here as they are scheduled and conducted</p>
        </div>
      ) : (
        <div className="space-y-4">
          {meetings.map((meeting) => (
            <div 
              key={meeting.id}
              className="bg-white rounded-lg shadow p-4 hover:shadow-md transition-shadow cursor-pointer"
              onClick={() => navigate(`/meetings/${meeting.id}`)}
            >
              <div className="flex items-start justify-between">
                <div className="flex-1">
                  <div className="flex items-center gap-2 mb-1">
                    <h3 className="font-semibold text-lg">{meeting.title || 'Untitled Meeting'}</h3>
                    <span className={`px-2 py-0.5 text-xs rounded capitalize ${typeColors[meeting.type] || 'bg-gray-100'}`}>
                      {meeting.type}
                    </span>
                    <span className={`px-2 py-0.5 text-xs rounded capitalize ${statusColors[meeting.status] || 'bg-gray-100'}`}>
                      {meeting.status.replace('_', ' ')}
                    </span>
                  </div>
                  <p className="text-sm text-gray-500">
                    {new Date(meeting.scheduled_at).toLocaleString()}
                  </p>
                  {meeting.summary && (
                    <p className="text-sm text-gray-600 mt-2 line-clamp-2">{meeting.summary}</p>
                  )}
                </div>
                <div className="flex gap-4 text-center text-sm">
                  <div>
                    <div className="font-bold text-primary-600">{meeting.decision_count}</div>
                    <div className="text-xs text-gray-500">Decisions</div>
                  </div>
                  <div>
                    <div className="font-bold text-blue-600">{meeting.dialog_count}</div>
                    <div className="text-xs text-gray-500">Dialog</div>
                  </div>
                  <div>
                    <div className="font-bold text-green-600">{meeting.attendee_count}</div>
                    <div className="text-xs text-gray-500">Attendees</div>
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// Meeting Detail Page
function MeetingDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [meeting, setMeeting] = useState<Meeting | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!id) return
    const fetchMeeting = async () => {
      try {
        const data = await api.getMeeting(id)
        setMeeting(data)
      } finally {
        setLoading(false)
      }
    }
    fetchMeeting()
  }, [id])

  const dialogTypeColors: Record<string, string> = {
    statement: 'bg-gray-100',
    question: 'bg-blue-100',
    answer: 'bg-green-100',
    motion: 'bg-purple-100',
    vote: 'bg-yellow-100',
    decision: 'bg-orange-100',
  }

  const roleColors: Record<string, string> = {
    chair: 'text-purple-600',
    member: 'text-blue-600',
    ceo: 'text-green-600',
    presenter: 'text-orange-600',
  }

  if (loading) {
    return <div className="flex items-center justify-center h-64"><Loader2 className="w-8 h-8 animate-spin text-primary-500" /></div>
  }

  if (!meeting) {
    return <div className="text-center text-gray-500 py-8">Meeting not found</div>
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <button 
          onClick={() => navigate('/meetings')}
          className="p-2 hover:bg-gray-100 rounded"
        >
          <ChevronLeft className="w-5 h-5" />
        </button>
        <div className="flex-1">
          <h1 className="text-2xl font-bold">{meeting.title || 'Meeting Details'}</h1>
          <div className="flex items-center gap-2 text-sm text-gray-500">
            <span className="capitalize">{meeting.type}</span>
            <span>•</span>
            <span>{new Date(meeting.scheduled_at).toLocaleString()}</span>
            <span>•</span>
            <span className="capitalize">{meeting.status.replace('_', ' ')}</span>
          </div>
        </div>
      </div>

      {/* Summary & Key Info */}
      {(meeting.summary || meeting.key_decisions?.length > 0 || meeting.action_items?.length > 0) && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
          {meeting.summary && (
            <div className="bg-white rounded-lg shadow p-4 lg:col-span-3">
              <h2 className="font-semibold mb-2">Summary</h2>
              <p className="text-gray-700">{meeting.summary}</p>
            </div>
          )}
          {meeting.key_decisions?.length > 0 && (
            <div className="bg-white rounded-lg shadow p-4">
              <h2 className="font-semibold mb-2 text-green-700">Key Decisions ({meeting.key_decisions.length})</h2>
              <ul className="space-y-1">
                {meeting.key_decisions.map((decision, i) => (
                  <li key={i} className="flex items-start gap-2 text-sm">
                    <CheckCircle className="w-4 h-4 text-green-500 mt-0.5 flex-shrink-0" />
                    <span>{decision}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {meeting.action_items?.length > 0 && (
            <div className="bg-white rounded-lg shadow p-4">
              <h2 className="font-semibold mb-2 text-blue-700">Action Items ({meeting.action_items.length})</h2>
              <ul className="space-y-1">
                {meeting.action_items.map((item, i) => (
                  <li key={i} className="flex items-start gap-2 text-sm">
                    <Target className="w-4 h-4 text-blue-500 mt-0.5 flex-shrink-0" />
                    <span>{item}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {meeting.attendees?.length > 0 && (
            <div className="bg-white rounded-lg shadow p-4">
              <h2 className="font-semibold mb-2 text-purple-700">Attendees ({meeting.attendees.length})</h2>
              <div className="flex flex-wrap gap-2">
                {meeting.attendees.map((attendee, i) => (
                  <span key={i} className="px-2 py-1 bg-purple-50 text-purple-700 rounded text-xs">
                    {attendee}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Decisions */}
      {meeting.decisions?.length > 0 && (
        <div className="bg-white rounded-lg shadow">
          <div className="px-4 py-3 border-b">
            <h2 className="font-semibold">Board Decisions ({meeting.decisions.length})</h2>
          </div>
          <div className="divide-y">
            {meeting.decisions.map((decision) => (
              <div key={decision.id} className="p-4">
                <div className="flex items-center justify-between mb-2">
                  <div className="flex items-center gap-2">
                    <h3 className="font-medium">{decision.subject}</h3>
                    <span className={`px-2 py-0.5 text-xs rounded ${decision.passed ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}`}>
                      {decision.decision}
                    </span>
                  </div>
                  <span className="text-sm text-gray-500">{Math.round(decision.pass_pct)}% approval</span>
                </div>
                <p className="text-sm text-gray-600 mb-3">{decision.description}</p>
                {decision.votes?.length > 0 && (
                  <div className="bg-gray-50 rounded p-3">
                    <div className="text-xs font-medium text-gray-500 mb-2">Votes</div>
                    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-2">
                      {decision.votes.map((vote, i) => (
                        <div key={i} className="text-xs bg-white rounded p-2">
                          <div className="flex items-center justify-between mb-1">
                            <span className="font-medium capitalize">{vote.member_id.replace('_', ' ')}</span>
                            <span className={`px-1.5 py-0.5 rounded ${
                              vote.vote === 'approve' ? 'bg-green-100 text-green-700' :
                              vote.vote === 'reject' ? 'bg-red-100 text-red-700' :
                              'bg-gray-100 text-gray-600'
                            }`}>{vote.vote}</span>
                          </div>
                          {vote.reasoning && (
                            <p className="text-gray-500 line-clamp-2">{vote.reasoning}</p>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Dialog */}
      <div className="bg-white rounded-lg shadow">
        <div className="px-4 py-3 border-b">
          <h2 className="font-semibold">Meeting Dialog ({meeting.dialog?.length || 0})</h2>
        </div>
        <div className="divide-y max-h-[600px] overflow-y-auto">
          {meeting.dialog && meeting.dialog.length > 0 ? (
            meeting.dialog.map((entry) => (
              <div key={entry.id} className="p-4">
                <div className="flex items-start gap-3">
                  <div className={`w-8 h-8 rounded-full flex items-center justify-center text-white text-xs font-bold ${
                    entry.role === 'chair' ? 'bg-purple-500' :
                    entry.role === 'ceo' ? 'bg-green-500' :
                    entry.role === 'presenter' ? 'bg-orange-500' :
                    'bg-blue-500'
                  }`}>
                    {entry.speaker.charAt(0).toUpperCase()}
                  </div>
                  <div className="flex-1">
                    <div className="flex items-center gap-2 mb-1">
                      <span className={`font-medium ${roleColors[entry.role] || 'text-gray-700'}`}>{entry.speaker}</span>
                      <span className={`px-1.5 py-0.5 text-xs rounded ${dialogTypeColors[entry.type] || 'bg-gray-100'}`}>
                        {entry.type}
                      </span>
                      <span className="text-xs text-gray-400">
                        {new Date(entry.timestamp).toLocaleTimeString()}
                      </span>
                    </div>
                    <p className="text-gray-700">{entry.content}</p>
                  </div>
                </div>
              </div>
            ))
          ) : (
            <div className="p-8 text-center text-gray-400">No dialog recorded</div>
          )}
        </div>
      </div>

      {/* Agenda */}
      {meeting.agenda?.length > 0 && (
        <div className="bg-white rounded-lg shadow">
          <div className="px-4 py-3 border-b">
            <h2 className="font-semibold">Agenda ({meeting.agenda.length} items)</h2>
          </div>
          <div className="divide-y">
            {meeting.agenda.map((item) => (
              <div key={item.id} className="p-3">
                <div className="flex items-center justify-between">
                  <div>
                    <span className="font-medium">{item.title}</span>
                    <span className="ml-2 text-xs text-gray-500 capitalize">{item.type}</span>
                  </div>
                  <span className="text-xs text-gray-400">Presenter: {item.presenter}</span>
                </div>
                {item.description && (
                  <p className="text-sm text-gray-600 mt-1">{item.description}</p>
                )}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

// Admin page
function AdminPage() {
  const [status, setStatus] = useState<SystemStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [resetting, setResetting] = useState(false)
  const [confirmReset, setConfirmReset] = useState(false)

  const fetchStatus = async () => {
    try {
      const data = await api.getSystemStatus()
      setStatus(data)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchStatus()
    const interval = setInterval(fetchStatus, 3000)
    return () => clearInterval(interval)
  }, [])

  const handlePause = async () => {
    await api.pauseCompany()
    fetchStatus()
  }

  const handleResume = async () => {
    await api.resumeCompany()
    fetchStatus()
  }

  const handleReset = async () => {
    if (!confirmReset) {
      setConfirmReset(true)
      return
    }
    setResetting(true)
    try {
      await api.resetOrganization()
      setConfirmReset(false)
      fetchStatus()
    } finally {
      setResetting(false)
    }
  }

  if (loading) {
    return <div className="flex items-center justify-center h-64"><Loader2 className="w-8 h-8 animate-spin text-primary-500" /></div>
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Administration</h1>

      {/* System Status */}
      <div className="bg-white rounded-lg shadow p-4">
        <h2 className="text-lg font-semibold mb-4">System Status</h2>
        <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
          {(['database', 'redis', 'storage', 'providers', 'organization'] as const).map(service => (
            <div key={service} className="text-center p-3 bg-gray-50 rounded">
              <div className={`w-4 h-4 rounded-full mx-auto mb-2 ${status?.[service] ? 'bg-green-500' : 'bg-red-500'}`} />
              <div className="text-sm font-medium capitalize">{service}</div>
              <div className="text-xs text-gray-500">{status?.[service] ? 'Connected' : 'Disconnected'}</div>
            </div>
          ))}
        </div>
      </div>

      {/* Company Controls */}
      <div className="bg-white rounded-lg shadow p-4">
        <h2 className="text-lg font-semibold mb-4">Company Controls</h2>
        <div className="flex items-center gap-4 mb-4">
          <div className="flex items-center gap-2">
            <span className="text-sm text-gray-600">Status:</span>
            <span className={`px-2 py-1 rounded text-sm font-medium ${
              status?.org_status === 'running' 
                ? 'bg-green-100 text-green-800' 
                : 'bg-yellow-100 text-yellow-800'
            }`}>
              {status?.org_status || 'Unknown'}
            </span>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-sm text-gray-600">Seeded:</span>
            <span className={`px-2 py-1 rounded text-sm ${
              status?.seeded ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-600'
            }`}>
              {status?.seeded ? 'Yes' : 'No'}
            </span>
          </div>
        </div>

        <div className="flex gap-3">
          {status?.org_status === 'running' ? (
            <button
              onClick={handlePause}
              className="px-4 py-2 bg-yellow-500 text-white rounded hover:bg-yellow-600 font-medium"
            >
              Pause Company
            </button>
          ) : (
            <button
              onClick={handleResume}
              className="px-4 py-2 bg-green-500 text-white rounded hover:bg-green-600 font-medium"
            >
              Resume Company
            </button>
          )}
        </div>
      </div>

      {/* Organization Stats */}
      {status?.org_stats && (
        <div className="bg-white rounded-lg shadow p-4">
          <h2 className="text-lg font-semibold mb-4">Organization Stats</h2>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div className="text-center p-3 bg-gray-50 rounded">
              <div className="text-2xl font-bold text-primary-600">{status.org_stats.total_employees}</div>
              <div className="text-sm text-gray-500">Total Employees</div>
            </div>
            <div className="text-center p-3 bg-gray-50 rounded">
              <div className="text-2xl font-bold text-blue-600">{status.org_stats.by_status?.working || 0}</div>
              <div className="text-sm text-gray-500">Working</div>
            </div>
            <div className="text-center p-3 bg-gray-50 rounded">
              <div className="text-2xl font-bold text-green-600">{status.org_stats.by_status?.idle || 0}</div>
              <div className="text-sm text-gray-500">Idle</div>
            </div>
            <div className="text-center p-3 bg-gray-50 rounded">
              <div className="text-2xl font-bold text-purple-600">{status.org_stats.divisions}</div>
              <div className="text-sm text-gray-500">Divisions</div>
            </div>
          </div>
        </div>
      )}

      {/* Seed Info */}
      {status?.seed && (
        <div className="bg-white rounded-lg shadow p-4">
          <h2 className="text-lg font-semibold mb-4">Current Seed</h2>
          <div className="space-y-2 text-sm">
            <div><span className="font-medium">Company:</span> {status.seed.company_name}</div>
            <div><span className="font-medium">Sector:</span> {status.seed.sector}</div>
            <div><span className="font-medium">Mission:</span> {status.seed.mission}</div>
            <div><span className="font-medium">Vision:</span> {status.seed.vision}</div>
          </div>
        </div>
      )}

      {/* Config Info */}
      {status?.config && (
        <div className="bg-white rounded-lg shadow p-4">
          <h2 className="text-lg font-semibold mb-4">Configuration</h2>
          <div className="grid grid-cols-3 gap-4 text-sm">
            <div><span className="font-medium">Server Port:</span> {status.config.server_port}</div>
            <div><span className="font-medium">WebSocket Port:</span> {status.config.websocket_port}</div>
            <div><span className="font-medium">Provider:</span> {status.default_provider}</div>
          </div>
        </div>
      )}

      {/* Danger Zone */}
      <div className="bg-red-50 border border-red-200 rounded-lg p-4">
        <h2 className="text-lg font-semibold text-red-800 mb-2">Danger Zone</h2>
        <p className="text-sm text-red-600 mb-4">
          Reset will destroy all employees, divisions, restructuring history, and seed configuration.
          This action cannot be undone.
        </p>
        <div className="flex items-center gap-3">
          <button
            onClick={handleReset}
            disabled={resetting}
            className={`px-4 py-2 rounded font-medium ${
              confirmReset 
                ? 'bg-red-600 text-white hover:bg-red-700' 
                : 'bg-red-100 text-red-700 hover:bg-red-200'
            }`}
          >
            {resetting ? (
              <span className="flex items-center gap-2">
                <Loader2 className="w-4 h-4 animate-spin" /> Resetting...
              </span>
            ) : confirmReset ? (
              'Click again to confirm'
            ) : (
              'Reset Everything'
            )}
          </button>
          {confirmReset && (
            <button
              onClick={() => setConfirmReset(false)}
              className="px-4 py-2 bg-gray-200 text-gray-700 rounded hover:bg-gray-300"
            >
              Cancel
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

// Products Pipeline Page
function ProductsPage() {
  const [pipelines, setPipelines] = useState<Pipeline[]>([])
  const [byStage, setByStage] = useState<Record<string, number>>({})
  const [selectedPipeline, setSelectedPipeline] = useState<Pipeline | null>(null)
  
  useEffect(() => {
    api.getPipelines().then(data => {
      setPipelines(data.pipelines || [])
      setByStage(data.by_stage || {})
    })
    
    // Refresh every 10 seconds
    const interval = setInterval(() => {
      api.getPipelines().then(data => {
        setPipelines(data.pipelines || [])
        setByStage(data.by_stage || {})
      })
    }, 10000)
    
    return () => clearInterval(interval)
  }, [])
  
  const stageColors: Record<string, string> = {
    ideation: 'bg-purple-100 text-purple-700',
    work_packet: 'bg-blue-100 text-blue-700',
    csuite_review: 'bg-yellow-100 text-yellow-700',
    board_vote: 'bg-orange-100 text-orange-700',
    execution_plan: 'bg-cyan-100 text-cyan-700',
    production: 'bg-teal-100 text-teal-700',
    final_review: 'bg-indigo-100 text-indigo-700',
    launched: 'bg-green-100 text-green-700',
    rejected: 'bg-red-100 text-red-700',
  }
  
  const stageNames: Record<string, string> = {
    ideation: 'Ideation',
    work_packet: 'Work Packet',
    csuite_review: 'C-Suite Review',
    board_vote: 'Board Vote',
    execution_plan: 'Execution Plan',
    production: 'Production',
    final_review: 'Final Review',
    launched: 'Launched',
    rejected: 'Rejected',
  }
  
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Product Pipeline</h1>
        <span className="text-sm text-gray-500">{pipelines.length} products</span>
      </div>
      
      {/* Stage summary */}
      <div className="grid grid-cols-3 md:grid-cols-5 lg:grid-cols-9 gap-2">
        {Object.entries(stageNames).map(([key, name]) => (
          <div key={key} className={`rounded p-3 text-center ${stageColors[key]?.replace('text-', 'bg-').replace('-700', '-50')}`}>
            <div className="text-2xl font-bold">{byStage[key] || 0}</div>
            <div className="text-[10px]">{name}</div>
          </div>
        ))}
      </div>
      
      {/* Pipeline list */}
      <div className="bg-white rounded-lg shadow">
        <div className="px-4 py-3 border-b">
          <h2 className="font-semibold">All Products</h2>
        </div>
        <div className="divide-y">
          {pipelines.length === 0 ? (
            <div className="p-8 text-center text-gray-500">
              No products yet. Seed the company to start generating product ideas.
            </div>
          ) : (
            pipelines.map(p => (
              <div 
                key={p.id} 
                className={`p-4 cursor-pointer hover:bg-gray-50 ${selectedPipeline?.id === p.id ? 'bg-blue-50' : ''}`}
                onClick={() => setSelectedPipeline(selectedPipeline?.id === p.id ? null : p)}
              >
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-3">
                      <span className="font-semibold">{p.name}</span>
                      <span className={`px-2 py-0.5 rounded text-xs font-medium ${stageColors[p.stage]}`}>
                        {stageNames[p.stage]}
                      </span>
                      {p.revision_count > 0 && (
                        <span className="text-xs text-orange-600">Rev {p.revision_count}</span>
                      )}
                    </div>
                    {p.idea && (
                      <div className="text-sm text-gray-600 mt-1">{p.idea.solution}</div>
                    )}
                    <div className="text-xs text-gray-400 mt-1">
                      {p.category} • {p.target_market} • Updated {new Date(p.updated_at).toLocaleString()}
                    </div>
                  </div>
                  {p.stage === 'launched' && (
                    <div className="flex gap-2" onClick={e => e.stopPropagation()}>
                      <a 
                        href={`/api/v1/org/pipelines/${p.id}/download`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="px-3 py-1 bg-blue-500 text-white rounded text-sm hover:bg-blue-600"
                      >
                        Preview
                      </a>
                      <a 
                        href={`/api/v1/org/pipelines/${p.id}/download?download=true`}
                        className="px-3 py-1 bg-green-500 text-white rounded text-sm hover:bg-green-600"
                      >
                        Download
                      </a>
                    </div>
                  )}
                </div>
                
                {selectedPipeline?.id === p.id && (
                  <div className="mt-4 p-4 bg-gray-50 rounded space-y-4">
                    {p.idea && (
                      <div>
                        <h4 className="font-medium text-sm mb-2">Executive Summary</h4>
                        <div className="text-sm space-y-2">
                          <p><strong>Problem:</strong> {p.idea.problem}</p>
                          <p><strong>Solution:</strong> {p.idea.solution}</p>
                          <p><strong>Value Prop:</strong> {p.idea.value_proposition}</p>
                          <p><strong>Target:</strong> {p.idea.target_customer}</p>
                          <p><strong>Revenue:</strong> {p.idea.revenue_model}</p>
                        </div>
                      </div>
                    )}
                    
                    {p.has_work_packet && (
                      <div className="text-sm">
                        <span className="text-green-600">✓</span> Work packet completed
                      </div>
                    )}
                    
                    {p.csuite_review && (
                      <div className="text-sm">
                        <strong>C-Suite:</strong> {p.csuite_review.approved ? '✓ Approved' : '✗ Needs Work'}
                        {p.csuite_review.feedback && (
                          <p className="text-gray-600 mt-1">{p.csuite_review.feedback}</p>
                        )}
                      </div>
                    )}
                    
                    {p.board_decision && (
                      <div className="text-sm">
                        <strong>Board:</strong> {p.board_decision.approved ? '✓ Approved' : '✗ Rejected'}
                        <span className="ml-2 text-gray-500">
                          ({p.board_decision.votes_for} for / {p.board_decision.votes_against} against)
                        </span>
                      </div>
                    )}
                    
                    {p.execution_plan && (
                      <div className="text-sm">
                        <strong>Execution Plan:</strong> {p.execution_plan.timeline} • {p.execution_plan.budget}
                      </div>
                    )}
                    
                    {p.stage === 'launched' && (
                      <div className="flex gap-3">
                        <a 
                          href={`/api/v1/org/pipelines/${p.id}/download`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600"
                        >
                          Preview in Browser
                        </a>
                        <a 
                          href={`/api/v1/org/pipelines/${p.id}/download?download=true`}
                          className="px-4 py-2 bg-green-500 text-white rounded hover:bg-green-600"
                        >
                          Download HTML
                        </a>
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}

// Connection status indicator
function ConnectionIndicator() {
  const { connected } = useWS()
  return (
    <div className="flex items-center gap-2 px-4 py-2 text-xs">
      <span className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`} />
      <span className="text-gray-400">{connected ? 'Live' : 'Connecting...'}</span>
    </div>
  )
}

// Main App
export default function App() {
  const location = useLocation()

  const navItems = [
    { path: '/', icon: Home, label: 'Dashboard' },
    { path: '/products', icon: Target, label: 'Products' },
    { path: '/seed', icon: Rocket, label: 'Seed' },
    { path: '/workflows', icon: Play, label: 'Workflows' },
    { path: '/runs', icon: Activity, label: 'Runs' },
    { path: '/org', icon: Building2, label: 'Organization' },
    { path: '/meetings', icon: BarChart3, label: 'Meetings' },
    { path: '/people', icon: Users, label: 'People' },
    { path: '/admin', icon: Settings, label: 'Admin' },
  ]

  return (
    <WSProvider>
    <div className="min-h-screen flex">
      {/* Sidebar */}
      <aside className="w-64 bg-gray-900 text-white flex flex-col">
        <div className="p-4 border-b border-gray-800">
          <h1 className="text-xl font-bold">AI Corporation</h1>
          <p className="text-xs text-gray-400">Workflow Engine</p>
        </div>
        <nav className="p-2 flex-1">
          {navItems.map(item => (
            <Link
              key={item.path}
              to={item.path}
              className={`flex items-center gap-3 px-4 py-2 rounded mb-1 ${
                location.pathname === item.path
                  ? 'bg-primary-600 text-white'
                  : 'text-gray-300 hover:bg-gray-800'
              }`}
            >
              <item.icon className="w-5 h-5" />
              {item.label}
            </Link>
          ))}
        </nav>
        <div className="border-t border-gray-800">
          <ConnectionIndicator />
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 p-6">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/products" element={<ProductsPage />} />
          <Route path="/seed" element={<SeedSetupPage />} />
          <Route path="/workflows" element={<Workflows />} />
          <Route path="/runs" element={<Runs />} />
          <Route path="/runs/:id" element={<RunDetail />} />
          <Route path="/org" element={<OrganizationPage />} />
          <Route path="/org/employee/:id" element={<EmployeeDetailPage />} />
          <Route path="/meetings" element={<MeetingsPage />} />
          <Route path="/meetings/:id" element={<MeetingDetailPage />} />
          <Route path="/people" element={<PeoplePage />} />
          <Route path="/admin" element={<AdminPage />} />
        </Routes>
      </main>
    </div>
    </WSProvider>
  )
}
