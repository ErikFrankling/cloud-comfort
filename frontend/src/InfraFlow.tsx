import { useEffect, useRef, useState } from 'react'
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
  useReactFlow,
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
  ResourceAmazonEC2Instance,
  ResourceAmazonEC2ElasticIPAddress,
  ResourceAmazonEC2AutoScaling,
  ResourceAmazonCloudFrontDownloadDistribution,
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
  cidr?: string
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
  aws_cloudfront_distribution:      ResourceAmazonCloudFrontDownloadDistribution as AwsIcon,
  aws_cloudfront_origin_access_control: ResourceAmazonCloudFrontDownloadDistribution as AwsIcon,
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

// ─── Descriptions ────────────────────────────────────────────────────────────
const resourceDescriptions: Record<string, string> = {
  aws_vpc:                          'Isolated virtual network in AWS. Defines the IP address range and network topology for your resources.',
  aws_subnet:                       'A range of IP addresses within a VPC. Resources in public subnets can be reached from the internet; private subnets cannot.',
  aws_internet_gateway:             'Enables communication between your VPC and the internet. Attach one to a VPC to allow public traffic.',
  aws_nat_gateway:                  'Allows private subnet resources to initiate outbound internet traffic without being directly reachable from the internet.',
  aws_route_table:                  'Contains routing rules that control where network traffic from subnets is directed.',
  aws_route_table_association:      'Associates a route table with a specific subnet, applying its routing rules to that subnet.',
  aws_security_group:               'Acts as a virtual firewall controlling inbound and outbound traffic for AWS resources using allow rules.',
  aws_security_group_rule:          'An individual inbound or outbound rule within a security group.',
  aws_network_acl:                  'Optional stateless firewall at the subnet level. Evaluates rules in order for inbound and outbound traffic.',
  aws_eip:                          'A static public IPv4 address that can be associated with an instance or NAT gateway.',
  aws_network_interface:            'A virtual network card that can be attached to an EC2 instance within a VPC.',
  aws_vpc_endpoint:                 'Privately connects your VPC to AWS services without requiring an internet gateway or NAT device.',
  aws_vpc_peering_connection:       'A networking connection between two VPCs enabling private routing between them.',
  aws_customer_gateway:             'Represents the on-premises side of a VPN connection to AWS.',
  aws_vpn_gateway:                  'The AWS-side anchor of a VPN connection, attached to a VPC.',
  aws_instance:                     'A virtual machine running in EC2. Choose instance type to balance compute, memory, and networking.',
  aws_launch_template:              'Reusable configuration for launching EC2 instances, including AMI, instance type, and security groups.',
  aws_launch_configuration:         'Legacy EC2 launch configuration used with Auto Scaling groups.',
  aws_autoscaling_group:            'Automatically adjusts the number of EC2 instances based on demand or a schedule.',
  aws_lambda_function:              'Serverless function that runs code in response to events without provisioning servers.',
  aws_lambda_permission:            'Grants an AWS service or account permission to invoke a Lambda function.',
  aws_ecs_cluster:                  'Logical grouping of ECS tasks or services. Can run on EC2 or Fargate.',
  aws_ecs_service:                  'Maintains a desired number of running ECS task instances and integrates with load balancers.',
  aws_ecs_task_definition:          'Blueprint for ECS tasks, defining container images, CPU/memory, networking, and IAM roles.',
  aws_eks_cluster:                  'Managed Kubernetes control plane. Runs the Kubernetes API server and etcd.',
  aws_eks_node_group:               'A group of EC2 instances registered as Kubernetes worker nodes in an EKS cluster.',
  aws_s3_bucket:                    'Object storage for any amount of data. Used for backups, static assets, data lakes, and more.',
  aws_s3_bucket_versioning:         'Enables versioning on an S3 bucket, preserving every version of every object.',
  aws_s3_bucket_policy:             'Resource-based policy that controls access to an S3 bucket and its objects.',
  aws_s3_bucket_public_access_block: 'Settings to block public access to an S3 bucket regardless of ACLs or bucket policies.',
  aws_db_instance:                  'A managed relational database instance (RDS). Supports MySQL, PostgreSQL, SQL Server, and more.',
  aws_db_subnet_group:              'Specifies which subnets an RDS instance can be placed in, controlling its network placement.',
  aws_rds_cluster:                  'A managed Aurora DB cluster with auto-scaling storage and multi-AZ replication.',
  aws_dynamodb_table:               'Fully managed NoSQL key-value and document database with single-digit millisecond performance.',
  aws_elasticache_cluster:          'Managed in-memory cache (Redis or Memcached) for speeding up database and application performance.',
  aws_elasticache_replication_group: 'A Redis replication group with a primary node and read replicas for high availability.',
  aws_redshift_cluster:             'Managed petabyte-scale data warehouse optimised for analytical queries.',
  aws_iam_role:                     'An identity with permissions that can be assumed by AWS services, users, or other accounts.',
  aws_iam_policy:                   'A document defining permissions that can be attached to IAM identities or resources.',
  aws_iam_user:                     'A permanent identity for a person or application interacting with AWS.',
  aws_iam_group:                    'A collection of IAM users that share the same permissions.',
  aws_iam_instance_profile:         'Container for an IAM role that can be attached to an EC2 instance.',
  aws_secretsmanager_secret:        'Stores and rotates sensitive information such as database passwords and API keys.',
  aws_lb:                           'Application Load Balancer distributing HTTP/HTTPS traffic across targets based on content.',
  aws_alb:                          'Application Load Balancer distributing HTTP/HTTPS traffic across targets based on content.',
  aws_lb_listener:                  'Checks for connection requests using a configured protocol and port, then routes to a target group.',
  aws_lb_target_group:              'Routes requests to registered targets (instances, IPs, Lambdas) using a health-checked algorithm.',
  aws_nlb:                          'Network Load Balancer handling millions of TCP/UDP requests per second at ultra-low latency.',
  aws_sqs_queue:                    'Fully managed message queue for decoupling and scaling microservices and distributed systems.',
  aws_cloudwatch_log_group:         'Container for CloudWatch log streams. Stores and organises log data from AWS services and apps.',
  aws_cloudwatch_metric_alarm:      'Watches a CloudWatch metric and triggers an action when it crosses a defined threshold.',
  aws_cognito_user_pool:            'User directory for authentication. Handles sign-up, sign-in, and token issuance for your app.',
  aws_cognito_identity_pool:        'Grants users temporary AWS credentials to access AWS services directly from your app.',
  aws_api_gateway_rest_api:         'Managed REST API gateway that routes HTTP requests to backend services like Lambda or EC2.',
  aws_api_gateway_v2_api:           'Managed HTTP or WebSocket API for low-latency, scalable API endpoints backed by Lambda or HTTP.',
  aws_route53_zone:                 'A container for DNS records for a domain, hosted in Route 53.',
  aws_route53_record:               'A DNS record within a Route 53 hosted zone (A, CNAME, MX, etc.).',
}

function getDescription(resourceType?: string): string {
  if (!resourceType) return ''
  if (resourceDescriptions[resourceType]) return resourceDescriptions[resourceType]
  // prefix fallbacks
  if (resourceType.startsWith('aws_s3'))          return resourceDescriptions.aws_s3_bucket
  if (resourceType.startsWith('aws_lambda'))      return resourceDescriptions.aws_lambda_function
  if (resourceType.startsWith('aws_ecs'))         return resourceDescriptions.aws_ecs_cluster
  if (resourceType.startsWith('aws_eks'))         return resourceDescriptions.aws_eks_cluster
  if (resourceType.startsWith('aws_iam'))         return resourceDescriptions.aws_iam_role
  if (resourceType.startsWith('aws_rds') || resourceType.startsWith('aws_db')) return resourceDescriptions.aws_db_instance
  if (resourceType.startsWith('aws_dynamodb'))    return resourceDescriptions.aws_dynamodb_table
  if (resourceType.startsWith('aws_elasticache')) return resourceDescriptions.aws_elasticache_cluster
  if (resourceType.startsWith('aws_cloudwatch'))  return resourceDescriptions.aws_cloudwatch_log_group
  if (resourceType.startsWith('aws_cognito'))     return resourceDescriptions.aws_cognito_user_pool
  if (resourceType.startsWith('aws_api_gateway')) return resourceDescriptions.aws_api_gateway_rest_api
  if (resourceType.startsWith('aws_sqs'))         return resourceDescriptions.aws_sqs_queue
  if (resourceType.startsWith('aws_route53'))     return resourceDescriptions.aws_route53_zone
  if (resourceType.startsWith('aws_lb') || resourceType.startsWith('aws_alb')) return resourceDescriptions.aws_lb
  if (resourceType.startsWith('aws_secretsmanager')) return resourceDescriptions.aws_secretsmanager_secret
  if (resourceType.startsWith('aws_autoscaling')) return resourceDescriptions.aws_autoscaling_group
  if (resourceType.startsWith('aws_security_group')) return resourceDescriptions.aws_security_group
  if (resourceType.startsWith('aws_route_table')) return resourceDescriptions.aws_route_table
  if (resourceType.startsWith('aws_nat_gateway')) return resourceDescriptions.aws_nat_gateway
  if (resourceType.startsWith('aws_internet_gateway')) return resourceDescriptions.aws_internet_gateway
  if (resourceType.startsWith('aws_subnet'))      return resourceDescriptions.aws_subnet
  if (resourceType.startsWith('aws_vpc'))         return resourceDescriptions.aws_vpc
  return ''
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
  if (resourceType.startsWith('aws_autoscaling'))      return ResourceAmazonEC2AutoScaling as AwsIcon
  if (resourceType.startsWith('aws_cloudfront'))       return ResourceAmazonCloudFrontDownloadDistribution as AwsIcon
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
function ResourceNode({ data, id }: NodeProps) {
  const d = data as any
  const s = catStyle[d.category] ?? catStyle.other
  const Icon = getIcon(d.resourceType)
  const description = getDescription(d.resourceType)
  const [tooltip, setTooltip] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const { setNodes } = useReactFlow()

  const onMouseEnter = () => {
    setNodes(nds => nds.map(n => n.id === id ? { ...n, zIndex: 1000 } : n))
    timer.current = setTimeout(() => setTooltip(true), 500)
  }
  const onMouseLeave = () => {
    setNodes(nds => nds.map(n => n.id === id ? { ...n, zIndex: 2 } : n))
    if (timer.current) clearTimeout(timer.current)
    setTooltip(false)
  }

  // extract just the resource name (after the colon)
  const name = d.label?.includes(': ') ? d.label.split(': ')[1] : d.label

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
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      <Handle type="target" position={Position.Top}
        style={{ background: s.border, width: 8, height: 8 }} />
      {Icon && <Icon size={22} />}
      <span>{d.label?.includes(': ') ? d.label.split(': ')[1] : d.label}</span>
      <Handle type="source" position={Position.Bottom}
        style={{ background: s.border, width: 8, height: 8 }} />

      {tooltip && (
        <div className="node-tooltip">
          <div className="node-tooltip-header">
            {Icon && <Icon size={20} />}
            <span className="node-tooltip-name">{name}</span>
          </div>
          <div className="node-tooltip-type">{d.resourceType}</div>
          {description && <p className="node-tooltip-desc">{description}</p>}
          <div className="node-tooltip-badge" style={{ background: s.border }}>
            {d.category}
          </div>
        </div>
      )}
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
        {d.cidr && (
          <span style={{ color: cfg.text, fontSize: 10, opacity: 0.7, marginLeft: 4 }}>
            ({d.cidr})
          </span>
        )}
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
          data: { label: byId[sid].label, category: 'subnet', cidr: byId[sid].cidr },
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
      data: { label: tg.label, category: tg.category, cidr: tg.cidr },
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
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const baseEdges = useRef<Edge[]>([])

  useEffect(() => {
    setSelectedId(null)
    setRfNodes(computeLayout(apiNodes, apiEdges))
    const edges = apiEdges.map(e => ({
      id: e.id,
      source: e.source,
      target: e.target,
      animated: true,
      style: { stroke: '#546e7a', strokeWidth: 1.5 },
    }))
    baseEdges.current = edges
    setRfEdges(edges)
  }, [apiNodes, apiEdges])

  // Apply highlight whenever selectedId changes
  useEffect(() => {
    if (!selectedId) {
      // reset
      setRfNodes(nds => nds.map(n => ({ ...n, style: { ...n.style, opacity: 1 } })))
      setRfEdges(baseEdges.current)
      return
    }

    const connectedEdges = baseEdges.current.filter(
      e => e.source === selectedId || e.target === selectedId
    )
    const connectedIds = new Set(
      connectedEdges.flatMap(e => [e.source, e.target])
    )

    setRfNodes(nds => nds.map(n => ({
      ...n,
      style: {
        ...n.style,
        opacity: n.id === selectedId || connectedIds.has(n.id) ? 1 : 0.2,
      },
    })))

    setRfEdges(baseEdges.current.map(e => {
      const isConnected = e.source === selectedId || e.target === selectedId
      return isConnected
        ? {
            ...e,
            animated: true,
            className: 'edge-shimmer',
            style: { stroke: '#8ab4f8', strokeWidth: 2.5 },
          }
        : { ...e, animated: false, style: { stroke: '#546e7a', strokeWidth: 1.5, opacity: 0.15 } }
    }))
  }, [selectedId])

  // ── Summary bar data ────────────────────────────────────────────────────────
  const resources = apiNodes.filter(n => n.kind === 'resource')
  const cats = ['networking', 'compute', 'storage', 'database', 'security', 'other'] as const
  const catLabels: Record<string, string> = {
    networking: 'Networking', compute: 'Compute', storage: 'Storage',
    database: 'Database', security: 'Security', other: 'Other',
  }
  const summary = cats
    .map(c => ({ cat: c, count: resources.filter(n => n.category === c).length }))
    .filter(x => x.count > 0)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {resources.length > 0 && (
        <div className="diagram-summary-bar">
          <span className="diagram-summary-total">{resources.length} resources</span>
          <span className="diagram-summary-divider" />
          {summary.map(({ cat, count }) => (
            <span key={cat} className="diagram-summary-item">
              <span className="diagram-summary-dot" style={{ background: catStyle[cat]?.border }} />
              {count} {catLabels[cat]}
            </span>
          ))}
        </div>
      )}
      <div style={{ flex: 1, position: 'relative' }}>
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
          onNodeClick={(_, node) => setSelectedId(id => id === node.id ? null : node.id)}
          onPaneClick={() => setSelectedId(null)}
        >
          <Background color="#1e2330" variant={BackgroundVariant.Dots} gap={20} />
          <Controls style={{ background: '#1a1d27', borderColor: '#2a2d35' }} />
        </ReactFlow>
      </div>
    </div>
  )
}
