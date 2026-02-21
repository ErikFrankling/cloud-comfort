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
  ResourceAmazonVPCInternetGateway,
  ResourceAmazonVPCNATGateway,
  ResourceAmazonVPCRouter,
  ResourceAmazonVPCNetworkAccessControlList,
  ResourceAmazonVPCElasticNetworkInterface,
  ResourceAmazonVPCEndpoints,
  ResourceAmazonVPCPeeringConnection,
  ResourceAmazonVPCCustomerGateway,
  ResourceAmazonVPCVPNGateway,
  ResourceAmazonVPCVirtualprivatecloudVPC,
  ResourceAmazonEC2Instance,
  ResourceAmazonEC2ElasticIPAddress,
  ResourceAmazonEC2AutoScaling,
  ResourceAmazonSimpleStorageServiceBucket,
  ResourceAmazonDynamoDBTable,
  ResourceAmazonElastiCacheCacheNode,
  ResourceAmazonCloudWatchLogs,
  ResourceAmazonCloudWatchAlarm,
  ResourceAmazonSimpleQueueServiceQueue,
  ResourceElasticLoadBalancingApplicationLoadBalancer,
  ResourceElasticLoadBalancingNetworkLoadBalancer,
  ResourceAWSLambdaLambdaFunction,
  ArchitectureGroupVirtualprivatecloudVPC,
  ArchitectureGroupPublicsubnet,
  ArchitectureServiceAmazonRDS,
  ArchitectureServiceAmazonEC2,
  ArchitectureServiceAWSLambda,
  ArchitectureServiceAmazonECSAnywhere,
  ArchitectureServiceAmazonEKSCloud,
  ArchitectureServiceAWSIAMIdentityCenter,
  ArchitectureServiceAWSSecretsManager,
  ArchitectureServiceAWSNetworkFirewall,
  ArchitectureServiceAmazonCognito,
  ArchitectureServiceAmazonAPIGateway,
  ArchitectureServiceAmazonRoute53,
  ArchitectureServiceAmazonRedshift,
  ArchitectureServiceElasticLoadBalancing,
  ArchitectureServiceAmazonSimpleQueueService,
  ArchitectureServiceAmazonEC2AutoScaling,
} from 'aws-react-icons'
import type { FC } from 'react'

type AwsIcon = FC<{ size?: number | string; [k: string]: any }>

export interface ApiNode {
  id: string
  label: string
  category: string
  kind: 'group' | 'resource'
  resourceType?: string
  parent?: string
}

const resourceIconMap: Record<string, AwsIcon> = {
  // Networking — VPC components
  aws_internet_gateway:             ResourceAmazonVPCInternetGateway as AwsIcon,
  aws_nat_gateway:                  ResourceAmazonVPCNATGateway as AwsIcon,
  aws_route_table:                  ResourceAmazonVPCRouter as AwsIcon,
  aws_route_table_association:      ResourceAmazonVPCRouter as AwsIcon,
  aws_network_acl:                  ResourceAmazonVPCNetworkAccessControlList as AwsIcon,
  aws_network_interface:            ResourceAmazonVPCElasticNetworkInterface as AwsIcon,
  aws_vpc_endpoint:                 ResourceAmazonVPCEndpoints as AwsIcon,
  aws_vpc_peering_connection:       ResourceAmazonVPCPeeringConnection as AwsIcon,
  aws_customer_gateway:             ResourceAmazonVPCCustomerGateway as AwsIcon,
  aws_vpn_gateway:                  ResourceAmazonVPCVPNGateway as AwsIcon,
  aws_eip:                          ResourceAmazonEC2ElasticIPAddress as AwsIcon,
  // Compute
  aws_instance:                     ResourceAmazonEC2Instance as AwsIcon,
  aws_launch_template:              ArchitectureServiceAmazonEC2 as AwsIcon,
  aws_launch_configuration:         ArchitectureServiceAmazonEC2 as AwsIcon,
  aws_autoscaling_group:            ResourceAmazonEC2AutoScaling as AwsIcon,
  aws_lambda_function:              ResourceAWSLambdaLambdaFunction as AwsIcon,
  aws_lambda_permission:            ArchitectureServiceAWSLambda as AwsIcon,
  aws_ecs_cluster:                  ArchitectureServiceAmazonECSAnywhere as AwsIcon,
  aws_ecs_service:                  ArchitectureServiceAmazonECSAnywhere as AwsIcon,
  aws_ecs_task_definition:          ArchitectureServiceAmazonECSAnywhere as AwsIcon,
  aws_eks_cluster:                  ArchitectureServiceAmazonEKSCloud as AwsIcon,
  aws_eks_node_group:               ArchitectureServiceAmazonEKSCloud as AwsIcon,
  // Storage
  aws_s3_bucket:                    ResourceAmazonSimpleStorageServiceBucket as AwsIcon,
  aws_s3_bucket_versioning:         ResourceAmazonSimpleStorageServiceBucket as AwsIcon,
  aws_s3_bucket_policy:             ResourceAmazonSimpleStorageServiceBucket as AwsIcon,
  aws_s3_bucket_public_access_block: ResourceAmazonSimpleStorageServiceBucket as AwsIcon,
  aws_s3_bucket_server_side_encryption_configuration: ResourceAmazonSimpleStorageServiceBucket as AwsIcon,
  // Database
  aws_db_instance:                  ArchitectureServiceAmazonRDS as AwsIcon,
  aws_db_subnet_group:              ArchitectureServiceAmazonRDS as AwsIcon,
  aws_rds_cluster:                  ArchitectureServiceAmazonRDS as AwsIcon,
  aws_dynamodb_table:               ResourceAmazonDynamoDBTable as AwsIcon,
  aws_elasticache_cluster:          ResourceAmazonElastiCacheCacheNode as AwsIcon,
  aws_elasticache_replication_group: ResourceAmazonElastiCacheCacheNode as AwsIcon,
  aws_redshift_cluster:             ArchitectureServiceAmazonRedshift as AwsIcon,
  // Security
  aws_security_group:               ArchitectureServiceAWSNetworkFirewall as AwsIcon,
  aws_security_group_rule:          ArchitectureServiceAWSNetworkFirewall as AwsIcon,
  // IAM
  aws_iam_role:                     ArchitectureServiceAWSIAMIdentityCenter as AwsIcon,
  aws_iam_policy:                   ArchitectureServiceAWSIAMIdentityCenter as AwsIcon,
  aws_iam_user:                     ArchitectureServiceAWSIAMIdentityCenter as AwsIcon,
  aws_iam_group:                    ArchitectureServiceAWSIAMIdentityCenter as AwsIcon,
  aws_iam_instance_profile:         ArchitectureServiceAWSIAMIdentityCenter as AwsIcon,
  aws_secretsmanager_secret:        ArchitectureServiceAWSSecretsManager as AwsIcon,
  // Networking — load balancers
  aws_lb:                           ResourceElasticLoadBalancingApplicationLoadBalancer as AwsIcon,
  aws_alb:                          ResourceElasticLoadBalancingApplicationLoadBalancer as AwsIcon,
  aws_lb_listener:                  ArchitectureServiceElasticLoadBalancing as AwsIcon,
  aws_lb_target_group:              ArchitectureServiceElasticLoadBalancing as AwsIcon,
  aws_nlb:                          ResourceElasticLoadBalancingNetworkLoadBalancer as AwsIcon,
  // Messaging / events
  aws_sqs_queue:                    ResourceAmazonSimpleQueueServiceQueue as AwsIcon,
  // Monitoring
  aws_cloudwatch_log_group:         ResourceAmazonCloudWatchLogs as AwsIcon,
  aws_cloudwatch_metric_alarm:      ResourceAmazonCloudWatchAlarm as AwsIcon,
  // Other
  aws_cognito_user_pool:            ArchitectureServiceAmazonCognito as AwsIcon,
  aws_cognito_identity_pool:        ArchitectureServiceAmazonCognito as AwsIcon,
  aws_api_gateway_rest_api:         ArchitectureServiceAmazonAPIGateway as AwsIcon,
  aws_api_gateway_v2_api:           ArchitectureServiceAmazonAPIGateway as AwsIcon,
  aws_route53_zone:                 ArchitectureServiceAmazonRoute53 as AwsIcon,
  aws_route53_record:               ArchitectureServiceAmazonRoute53 as AwsIcon,
}

function getIcon(resourceType?: string): AwsIcon | null {
  if (!resourceType) return null
  if (resourceIconMap[resourceType]) return resourceIconMap[resourceType]
  // prefix fallbacks
  if (resourceType.startsWith('aws_internet_gateway')) return ResourceAmazonVPCInternetGateway as AwsIcon
  if (resourceType.startsWith('aws_nat_gateway'))      return ResourceAmazonVPCNATGateway as AwsIcon
  if (resourceType.startsWith('aws_route_table'))      return ResourceAmazonVPCRouter as AwsIcon
  if (resourceType.startsWith('aws_s3'))               return ResourceAmazonSimpleStorageServiceBucket as AwsIcon
  if (resourceType.startsWith('aws_lambda'))           return ResourceAWSLambdaLambdaFunction as AwsIcon
  if (resourceType.startsWith('aws_ecs'))              return ArchitectureServiceAmazonECSAnywhere as AwsIcon
  if (resourceType.startsWith('aws_eks'))              return ArchitectureServiceAmazonEKSCloud as AwsIcon
  if (resourceType.startsWith('aws_iam'))              return ArchitectureServiceAWSIAMIdentityCenter as AwsIcon
  if (resourceType.startsWith('aws_rds') || resourceType.startsWith('aws_db')) return ArchitectureServiceAmazonRDS as AwsIcon
  if (resourceType.startsWith('aws_dynamodb'))         return ResourceAmazonDynamoDBTable as AwsIcon
  if (resourceType.startsWith('aws_elasticache'))      return ResourceAmazonElastiCacheCacheNode as AwsIcon
  if (resourceType.startsWith('aws_cloudwatch'))       return ResourceAmazonCloudWatchLogs as AwsIcon
  if (resourceType.startsWith('aws_cognito'))          return ArchitectureServiceAmazonCognito as AwsIcon
  if (resourceType.startsWith('aws_api_gateway'))      return ArchitectureServiceAmazonAPIGateway as AwsIcon
  if (resourceType.startsWith('aws_sqs'))              return ResourceAmazonSimpleQueueServiceQueue as AwsIcon
  if (resourceType.startsWith('aws_route53'))          return ArchitectureServiceAmazonRoute53 as AwsIcon
  if (resourceType.startsWith('aws_lb') || resourceType.startsWith('aws_alb') || resourceType.startsWith('aws_elb')) return ResourceElasticLoadBalancingApplicationLoadBalancer as AwsIcon
  if (resourceType.startsWith('aws_secretsmanager'))   return ArchitectureServiceAWSSecretsManager as AwsIcon
  if (resourceType.startsWith('aws_autoscaling'))      return ArchitectureServiceAmazonEC2AutoScaling as AwsIcon
  if (resourceType.startsWith('aws_instance'))         return ResourceAmazonEC2Instance as AwsIcon
  if (resourceType.startsWith('aws_vpc'))              return ResourceAmazonVPCEndpoints as AwsIcon
  return null
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
      {Icon && <Icon size={22} />}
      <span>{d.label}</span>
      <Handle type="source" position={Position.Bottom}
        style={{ background: s.border, width: 8, height: 8 }} />
    </div>
  )
}

const groupIcons: Record<string, AwsIcon> = {
  vpc:    ArchitectureGroupVirtualprivatecloudVPC as AwsIcon,
  subnet: ArchitectureGroupPublicsubnet as AwsIcon,
}

function GroupNode({ data }: NodeProps) {
  const d = data as any
  const cfg = groupCfg[d.category] ?? groupCfg.global
  const Icon = groupIcons[d.category] ?? null
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
      <div style={{
        position: 'absolute', top: 6, left: 10,
        display: 'flex', alignItems: 'center', gap: 5,
        pointerEvents: 'none',
      }}>
        {Icon && <Icon size={18} />}
        <span style={{ color: cfg.text, fontSize: 12, fontWeight: 700, letterSpacing: '0.01em' }}>
          {d.label}
        </span>
      </div>
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
          data: { label: byId[rid].label, category: byId[rid].category, resourceType: byId[rid].resourceType },
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
          data: { label: byId[rid].label, category: byId[rid].category, resourceType: byId[rid].resourceType },
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
