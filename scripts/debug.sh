#!/bin/bash

# ==== User-defined inputs ====
EAST_REGION="us-east-2"
WEST_REGION="us-west-2"
EAST_VPC_CIDR="172.16.0.0/16"
WEST_VPC_CIDR="172.17.0.0/16"
EAST_INSTANCE_IP="172.16.1.44"
WEST_INSTANCE_IP="172.17.1.12"

    # eastInstanceId       : "i-075cbf133236c2158"
    # eastInstancePrivateIP: "172.16.1.44"
    # eastSubnet           : "subnet-06b1e21dffdf96994"
    # eastTGW              : "tgw-0920e4ac8f40c64ab"
    # eastTGWRt            : "tgw-rtb-0a0501507ec786fc8"
    # westInstanceId       : "i-085d5abbde93ab2c5"
    # westInstancePrivateIP: "172.17.1.12"
    # westInstanceSGId     : "sg-0a31d000f17f3c9ce"
    # westSubnet           : "subnet-082774a846bfdc560"
    # westTGW              : "tgw-0a1cf0163ddaed544"
    # westTGWRt            : "tgw-rtb-076dd099eecad9e69"

EAST_TGW_ID="tgw-0920e4ac8f40c64ab"
EAST_SUBNET_ID="subnet-06b1e21dffdf96994"
WEST_TGW_ID="tgw-0a1cf0163ddaed544"
WEST_SUBNET_ID="subnet-082774a846bfdc560"
WEST_INSTANCE_SG_ID="sg-0a31d000f17f3c9ce"
EFLOW_LOG_GROUP="neweastregionblogvpc"
FLOW_LOG_GROUP="newwestregionblogvpc"  # only if VPC flow logs enabled

# ==== Auto-resolve TGW route table IDs ====
echo "🔄 Getting East TGW Route Table ID..."
EAST_TGW_RT_ID=$(aws ec2 describe-transit-gateway-route-tables \
  --region $EAST_REGION \
  --filters Name=transit-gateway-id,Values=$EAST_TGW_ID \
  --query 'TransitGatewayRouteTables[0].TransitGatewayRouteTableId' \
  --output text)

echo "🔄 Getting West TGW Route Table ID..."
WEST_TGW_RT_ID=$(aws ec2 describe-transit-gateway-route-tables \
  --region $WEST_REGION \
  --filters Name=transit-gateway-id,Values=$WEST_TGW_ID \
  --query 'TransitGatewayRouteTables[0].TransitGatewayRouteTableId' \
  --output text)

echo -e "\n📘 East TGW Route Table: $EAST_TGW_RT_ID"
echo -e "📘 West TGW Route Table: $WEST_TGW_RT_ID\n"

# ==== Begin route and config validation ====

echo "🔍 Checking East TGW Route Table for West CIDR..."
aws ec2 search-transit-gateway-routes \
  --region $EAST_REGION \
  --transit-gateway-route-table-id $EAST_TGW_RT_ID \
  --filters Name=route-search.subnet-of-match,Values=$WEST_VPC_CIDR \
  --query "Routes[].[DestinationCidrBlock,State,TransitGatewayAttachments[0].ResourceType]" \
  --output table

echo -e "\n🔍 Checking West TGW Route Table for East CIDR..."
aws ec2 search-transit-gateway-routes \
  --region $WEST_REGION \
  --transit-gateway-route-table-id $WEST_TGW_RT_ID \
  --filters Name=route-search.subnet-of-match,Values=$EAST_VPC_CIDR \
  --query "Routes[].[DestinationCidrBlock,State,TransitGatewayAttachments[0].ResourceType]" \
  --output table

echo -e "\n📡 Checking East Subnet Route Table for route to West VPC..."
aws ec2 describe-route-tables \
  --region $EAST_REGION \
  --filters Name=association.subnet-id,Values=$EAST_SUBNET_ID \
  --query "RouteTables[].Routes[?DestinationCidrBlock=='$WEST_VPC_CIDR']" \
  --output table

echo -e "\n📡 Checking West Subnet Route Table for route to East VPC..."
aws ec2 describe-route-tables \
  --region $WEST_REGION \
  --filters Name=association.subnet-id,Values=$WEST_SUBNET_ID \
  --query "RouteTables[].Routes[?DestinationCidrBlock=='$EAST_VPC_CIDR']" \
  --output table

echo -e "\n🔐 Checking Security Group of West Instance..."
aws ec2 describe-security-groups \
  --region $WEST_REGION \
  --group-ids $WEST_INSTANCE_SG_ID \
  --query "SecurityGroups[].IpPermissions[?contains(IpRanges[].CidrIp, '$EAST_VPC_CIDR')]" \
  --output table

# echo -e "\n📊 Checking VPC Flow Logs for West Instance IP (if enabled)..."
aws logs filter-log-events \
  --region $EAST_REGION \
  --log-group-name "$EFLOW_LOG_GROUP" \
  --filter-pattern "\"$EAST_INSTANCE_IP\"" \
  --limit 10

# echo -e "\n📊 Checking VPC Flow Logs for West Instance IP (if enabled)..."
aws logs filter-log-events \
  --region $WEST_REGION \
  --log-group-name "$FLOW_LOG_GROUP" \
  --filter-pattern "\"$WEST_INSTANCE_IP\"" \
  --limit 10