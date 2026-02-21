import { useEffect } from 'react'
import dagre from '@dagrejs/dagre'
import {
  ReactFlow,
  Background,
  Controls,
  Node,
  Edge,
  Position,
  Handle,
  NodeProps,
  BackgroundVariant,
  useNodesState,
  useEdgesState,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import {
  SiAmazons3,
  SiAmazonec2,
  SiAwslambda,
  SiAmazonrds,
  SiAmazondynamodb,
  SiAmazonecs,
  SiAmazoneks,
  SiAmazonelasticache,
  SiAmazoniam,
  SiAmazoncloudwatch,
  SiAmazoncognito,
  SiAmazonapigateway,
  SiAmazonsqs,
  SiAmazonredshift,
  SiAmazonroute53,
  SiAwselasticloadbalancing,
  SiAwsfargate,
  SiAwssecretsmanager,
  SiAmazonwebservices,
} from 'react-icons/si'
import type { IconType } from 'react-icons'

export interface ApiNode {
  id: string
  label: string
  category: string
  kind: 'group' | 'resource'
  resourceType?: string
  parent?: string
}

const resourceIcons: Record<string, IconType> = {
  aws_s3_bucket:                               SiAmazons3,
  aws_s3_bucket_versioning:                    SiAmazons3,
  aws_s3_bucket_policy:                        SiAmazons3,
  aws_s3_bucket_public_access_block:           SiAmazons3,
  aws_s3_bucket_server_side_encryption_configuration: SiAmazons3,
  aws_instance:                                SiAmazonec2,
  aws_launch_template:                         SiAmazonec2,
  aws_launch_configuration:                    SiAmazonec2,
  aws_autoscaling_group:                       SiAmazonec2,
  aws_lambda_function:                         SiAwslambda,
  aws_lambda_permission:                       SiAwslambda,
  aws_db_instance:                             SiAmazonrds,
  aws_db_subnet_group:                         SiAmazonrds,
  aws_rds_cluster:                             SiAmazonrds,
  aws_dynamodb_table:                          SiAmazondynamodb,
  aws_ecs_cluster:                             SiAmazonecs,
  aws_ecs_service:                             SiAmazonecs,
  aws_ecs_task_definition:                     SiAmazonecs,
  aws_eks_cluster:                             SiAmazoneks,
  aws_eks_node_group:                          SiAmazoneks,
  aws_elasticache_cluster:                     SiAmazonelasticache,
  aws_elasticache_replication_group:           SiAmazonelasticache,
  aws_iam_role:                                SiAmazoniam,
  aws_iam_policy:                              SiAmazoniam,
  aws_iam_user:                                SiAmazoniam,
  aws_iam_group:                               SiAmazoniam,
  aws_iam_instance_profile:                    SiAmazoniam,
  aws_cloudwatch_log_group:                    SiAmazoncloudwatch,
  aws_cloudwatch_metric_alarm:                 SiAmazoncloudwatch,
  aws_cognito_user_pool:                       SiAmazoncognito,
  aws_cognito_identity_pool:                   SiAmazoncognito,
  aws_api_gateway_rest_api:                    SiAmazonapigateway,
  aws_api_gateway_v2_api:                      SiAmazonapigateway,
  aws_sqs_queue:                               SiAmazonsqs,
  aws_redshift_cluster:                        SiAmazonredshift,
  aws_route53_zone:                            SiAmazonroute53,
  aws_route53_record:                          SiAmazonroute53,
  aws_lb:                                      SiAwselasticloadbalancing,
  aws_alb:                                     SiAwselasticloadbalancing,
  aws_lb_listener:                             SiAwselasticloadbalancing,
  aws_lb_target_group:                         SiAwselasticloadbalancing,
  aws_fargate_profile:                         SiAwsfargate,
  aws_secretsmanager_secret:                   SiAwssecretsmanager,
}

function getIcon(resourceType?: string): IconType | null {
  if (!resourceType) return null
  if (resourceIcons[resourceType]) return resourceIcons[resourceType]
  if (resourceType.startsWith('aws_s3'))            return SiAmazons3
  if (resourceType.startsWith('aws_lambda'))        return SiAwslambda
  if (resourceType.startsWith('aws_ecs'))           return SiAmazonecs
  if (resourceType.startsWith('aws_eks'))           return SiAmazoneks
  if (resourceType.startsWith('aws_iam'))           return SiAmazoniam
  if (resourceType.startsWith('aws_rds') || resourceType.startsWith('aws_db')) return SiAmazonrds
  if (resourceType.startsWith('aws_dynamodb'))      return SiAmazondynamodb
  if (resourceType.startsWith('aws_elasticache'))   return SiAmazonelasticache
  if (resourceType.startsWith('aws_cloudwatch'))    return SiAmazoncloudwatch
  if (resourceType.startsWith('aws_cognito'))       return SiAmazoncognito
  if (resourceType.startsWith('aws_api_gateway'))   return SiAmazonapigateway
  if (resourceType.startsWith('aws_sqs'))           return SiAmazonsqs
  if (resourceType.startsWith('aws_route53'))       return SiAmazonroute53
  if (resourceType.startsWith('aws_lb') || resourceType.startsWith('aws_alb') || resourceType.startsWith('aws_elb')) return SiAwselasticloadbalancing
  if (resourceType.startsWith('aws_secretsmanager')) return SiAwssecretsmanager
  return SiAmazonwebservices
}

export interface ApiEdge {
  id: string
  source: string
  target: string
}

// ─── Layout constants ────────────────────────────────────────────────────────
const NODE_W = 180
const NODE_H = 72
const GROUP_PAD = 24
const GROUP_HEADER = 36
const GROUP_GAP = 48

// ─── Styles ─────────────────────────────────────────────────────────────────
const catStyle: Record<string, { bg: string; border: string; text: string }> = {
  networking: { bg: '#0d47a1', border: '#42a5f5', text: '#e3f2fd' },
  compute:    { bg: '#bf360c', border: '#ff7043', text: '#fbe9e7' },
  storage:    { bg: '#1b5e20', border: '#66bb6a', text: '#e8f5e9' },
  database:   { bg: '#4a148c', border: '#ab47bc', text: '#f3e5f5' },
  security:   { bg: '#b71c1c', border: '#ef5350', text: '#ffebee' },
  other:      { bg: '#263238', border: '#78909c', text: '#eceff1' },
}

const groupCfg: Record<string, { bg: string; border: string; text: string; dashed?: boolean }> = {
  vpc:    { bg: 'rgba(13,71,161,0.15)',  border: '#42a5f5', text: '#e3f2fd' },
  subnet: { bg: 'rgba(13,71,161,0.09)',  border: '#90caf9', text: '#bbdefb', dashed: true },
  global: { bg: 'rgba(55,71,79,0.18)',   border: '#78909c', text: '#eceff1', dashed: true },
}

// ─── Custom nodes ────────────────────────────────────────────────────────────
function ResourceNode({ data }: NodeProps) {
  const d = data as any
  const s = catStyle[d.category] ?? catStyle.other
  const Icon = getIcon(d.resourceType)
  return (
    <div
      className="resource-node-inner"
      style={{
        background: s.bg,
        border: `2px solid ${s.border}`,
        borderRadius: 6,
        padding: '6px 10px',
        color: s.text,
        fontSize: 11,
        fontWeight: 600,
        width: NODE_W,
        textAlign: 'center',
        lineHeight: 1.35,
        boxShadow: '0 2px 8px rgba(0,0,0,0.45)',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        gap: 4,
        ['--node-border' as any]: s.border,
      }}
    >
      <Handle type="target" position={Position.Top}
        style={{ background: s.border, width: 8, height: 8 }} />
      {Icon && <Icon size={18} color={s.border} />}
      <span>{d.label}</span>
      <Handle type="source" position={Position.Bottom}
        style={{ background: s.border, width: 8, height: 8 }} />
    </div>
  )
}

function GroupNode({ data }: NodeProps) {
  const cfg = groupCfg[(data as any).category] ?? groupCfg.global
  return (
    <div style={{
      width: '100%', height: '100%',
      background: cfg.bg,
      border: `${cfg.dashed ? '1.5px dashed' : '2px solid'} ${cfg.border}`,
      borderRadius: 8,
      boxSizing: 'border-box',
      position: 'relative',
      pointerEvents: 'none',
    }}>
      <span style={{
        position: 'absolute', top: 9, left: 12,
        color: cfg.text, fontSize: 12, fontWeight: 700,
        letterSpacing: '0.01em', pointerEvents: 'none',
      }}>
        {(data as any).label}
      </span>
    </div>
  )
}

const nodeTypes = { resource: ResourceNode, infraGroup: GroupNode }

// ─── Dagre layout helper ─────────────────────────────────────────────────────
// Lays out a set of resource nodes using their edges and returns positions
// normalised to start at (0, 0). Returns the bounding box dimensions too.
function dagreLayout(
  ids: string[],
  edges: ApiEdge[],
): { pos: Record<string, { x: number; y: number }>; w: number; h: number } {
  if (ids.length === 0) return { pos: {}, w: 0, h: 0 }

  const g = new dagre.graphlib.Graph()
  g.setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: 'TB', ranksep: 50, nodesep: 36, marginx: 0, marginy: 0 })

  for (const id of ids) g.setNode(id, { width: NODE_W, height: NODE_H })

  const idSet = new Set(ids)
  for (const e of edges) {
    if (idSet.has(e.source) && idSet.has(e.target)) {
      g.setEdge(e.source, e.target)
    }
  }

  dagre.layout(g)

  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
  const raw: Record<string, { x: number; y: number }> = {}
  for (const id of ids) {
    const n = g.node(id)
    if (!n) continue
    const x = n.x - NODE_W / 2
    const y = n.y - NODE_H / 2
    raw[id] = { x, y }
    minX = Math.min(minX, x);      minY = Math.min(minY, y)
    maxX = Math.max(maxX, x + NODE_W); maxY = Math.max(maxY, y + NODE_H)
  }

  const pos: Record<string, { x: number; y: number }> = {}
  for (const id of ids) {
    if (raw[id]) pos[id] = { x: raw[id].x - minX, y: raw[id].y - minY }
  }

  return { pos, w: maxX - minX, h: maxY - minY }
}

// ─── Full layout ─────────────────────────────────────────────────────────────
function computeLayout(apiNodes: ApiNode[], apiEdges: ApiEdge[]): Node[] {
  const byId: Record<string, ApiNode> = {}
  for (const n of apiNodes) byId[n.id] = n

  const childrenOf: Record<string, string[]> = {}
  for (const n of apiNodes) {
    if (n.parent) {
      if (!childrenOf[n.parent]) childrenOf[n.parent] = []
      childrenOf[n.parent].push(n.id)
    }
  }

  const resKids  = (id: string) => (childrenOf[id] ?? []).filter(k => byId[k]?.kind === 'resource')
  const grpKids  = (id: string) => (childrenOf[id] ?? []).filter(k => byId[k]?.kind === 'group')

  // Collect groups and resources separately so parents always precede children
  const groupNodes: Node[] = []
  const resourceNodes: Node[] = []

  // All top-level groups, placed left-to-right
  const topGroups = apiNodes.filter(n => n.kind === 'group' && !n.parent)
  let topX = 0

  for (const tg of topGroups) {
    let tgW = 0, tgH = 0

    if (tg.category === 'vpc') {
      // ── VPC: subnets in a row, VPC-level resources below ───────────────────
      const subnets   = grpKids(tg.id).filter(id => byId[id]?.category === 'subnet')
      const vpcDirect = resKids(tg.id)

      let subX = GROUP_PAD
      let subRowH = 0

      for (const sid of subnets) {
        const sResIds = resKids(sid)
        const { pos, w, h } = dagreLayout(sResIds, apiEdges)
        const sw = Math.max(w + GROUP_PAD * 2, NODE_W + GROUP_PAD * 2)
        const sh = h + GROUP_PAD * 2 + GROUP_HEADER
        subRowH = Math.max(subRowH, sh)

        groupNodes.push({
          id: sid, type: 'infraGroup',
          parentId: tg.id, extent: 'parent',
          position: { x: subX, y: GROUP_HEADER + GROUP_PAD },
          data: { label: byId[sid].label, category: 'subnet' },
          style: { width: sw, height: sh },
          zIndex: 1, selectable: false,
        } as Node)

        for (const rid of sResIds) {
          if (!pos[rid]) continue
          resourceNodes.push({
            id: rid, type: 'resource',
            parentId: sid, extent: 'parent',
            position: { x: GROUP_PAD + pos[rid].x, y: GROUP_HEADER + GROUP_PAD + pos[rid].y },
            data: { label: byId[rid].label, category: byId[rid].category, resourceType: byId[rid].resourceType },
            zIndex: 2,
          } as Node)
        }

        subX += sw + GROUP_GAP
      }

      // VPC-direct resources below subnets
      const { pos: vPos, w: vW, h: vH } = dagreLayout(vpcDirect, apiEdges)
      const directY = GROUP_HEADER + GROUP_PAD + subRowH + (subRowH > 0 ? GROUP_GAP : 0)

      for (const rid of vpcDirect) {
        if (!vPos[rid]) continue
        resourceNodes.push({
          id: rid, type: 'resource',
          parentId: tg.id, extent: 'parent',
          position: { x: GROUP_PAD + vPos[rid].x, y: directY + vPos[rid].y },
          data: { label: byId[rid].label, category: byId[rid].category },
          zIndex: 2,
        } as Node)
      }

      tgW = Math.max(subX - GROUP_GAP + GROUP_PAD, vW + GROUP_PAD * 2, NODE_W + GROUP_PAD * 2)
      tgH = directY + (vH > 0 ? vH + GROUP_PAD : GROUP_PAD)

    } else {
      // ── Global / flat group ─────────────────────────────────────────────────
      const resIds = resKids(tg.id)
      const { pos, w, h } = dagreLayout(resIds, apiEdges)
      tgW = Math.max(w + GROUP_PAD * 2, NODE_W + GROUP_PAD * 2)
      tgH = h + GROUP_PAD * 2 + GROUP_HEADER

      for (const rid of resIds) {
        if (!pos[rid]) continue
        resourceNodes.push({
          id: rid, type: 'resource',
          parentId: tg.id, extent: 'parent',
          position: { x: GROUP_PAD + pos[rid].x, y: GROUP_HEADER + GROUP_PAD + pos[rid].y },
          data: { label: byId[rid].label, category: byId[rid].category },
          zIndex: 2,
        } as Node)
      }
    }

    // Top-level group node — pushed before its children
    groupNodes.unshift({
      id: tg.id, type: 'infraGroup',
      position: { x: topX, y: 0 },
      data: { label: tg.label, category: tg.category },
      style: { width: tgW || 300, height: tgH || 200 },
      zIndex: 0, selectable: false,
    } as Node)

    topX += (tgW || 300) + GROUP_GAP
  }

  // Parents (groups) before children (resources) for correct React Flow rendering
  return [...groupNodes, ...resourceNodes]
}

// ─── Component ───────────────────────────────────────────────────────────────
interface InfraFlowProps {
  nodes: ApiNode[]
  edges: ApiEdge[]
}

export default function InfraFlow({ nodes: apiNodes, edges: apiEdges }: InfraFlowProps) {
  const [rfNodes, setRfNodes, onNodesChange] = useNodesState<Node>([])
  const [rfEdges, setRfEdges, onEdgesChange] = useEdgesState<Edge>([])

  useEffect(() => {
    setRfNodes(computeLayout(apiNodes, apiEdges))
    setRfEdges(
      apiEdges.map(e => ({
        id: e.id,
        source: e.source,
        target: e.target,
        style: { stroke: '#546e7a', strokeWidth: 1.5 },
      }))
    )
  }, [apiNodes, apiEdges])

  return (
    <ReactFlow
      nodes={rfNodes}
      edges={rfEdges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      nodeTypes={nodeTypes}
      fitView
      fitViewOptions={{ padding: 0.12 }}
      minZoom={0.05}
      nodesDraggable={false}
      style={{ background: '#0d0f14' }}
    >
      <Background color="#1e2330" variant={BackgroundVariant.Dots} gap={20} />
      <Controls style={{ background: '#1a1d27', borderColor: '#2a2d35' }} />
    </ReactFlow>
  )
}
