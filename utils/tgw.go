package utils

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2transitgateway"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type TGWAttachmentArgs struct {
	Name    string
	Tgw     *ec2transitgateway.TransitGateway
	Vpc     *ec2.Vpc
	Subnets []pulumi.StringInput
}

func CreateTransitGateway(ctx *pulumi.Context, name string, asn int, opts ...pulumi.ResourceOption) (*ec2transitgateway.TransitGateway, *ec2transitgateway.RouteTable, error) {
	tgw, err := ec2transitgateway.NewTransitGateway(ctx, name, &ec2transitgateway.TransitGatewayArgs{
		AmazonSideAsn:                pulumi.Int(asn),
		AutoAcceptSharedAttachments:  pulumi.String("enable"),
		DefaultRouteTableAssociation: pulumi.String("disable"),
		DefaultRouteTablePropagation: pulumi.String("disable"),
		Tags: pulumi.StringMap{
			"Name": pulumi.String(name),
		},
	}, opts...)
	if err != nil {
		return nil, nil, err
	}

	rt, err := ec2transitgateway.NewRouteTable(ctx, name+"-rt", &ec2transitgateway.RouteTableArgs{
		TransitGatewayId: tgw.ID(),
		Tags: pulumi.StringMap{
			"Name": pulumi.String(name + "-rt"),
		},
	}, opts...)
	if err != nil {
		return nil, nil, err
	}

	return tgw, rt, nil
}

func AttachVPCsToTGW(
	ctx *pulumi.Context,
	tgw *ec2transitgateway.TransitGateway,
	tgwRt *ec2transitgateway.RouteTable,
	region string,
	vpcs []*VPCResult,
	opts ...pulumi.ResourceOption,
) ([]*ec2transitgateway.VpcAttachment, error) {
	var attachments []*ec2transitgateway.VpcAttachment
	for i, vpc := range vpcs {
		var subnetIds pulumi.StringArray
		for _, s := range vpc.PrivateTgwSubnets {
			subnetIds = append(subnetIds, s.ID())
		}
		dependsOnAndRegionOpts := append([]pulumi.ResourceOption{pulumi.DependsOn([]pulumi.Resource{tgw})}, opts...)
		attachment, err := ec2transitgateway.NewVpcAttachment(ctx, fmt.Sprintf("%s-vpc-tgw-attachment-%d", region, i), &ec2transitgateway.VpcAttachmentArgs{
			VpcId:            vpc.Vpc.ID(),
			TransitGatewayId: tgw.ID(),
			SubnetIds:        subnetIds,
			TransitGatewayDefaultRouteTableAssociation: pulumi.Bool(false),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-attachment-%d", region, i)),
			},
		}, dependsOnAndRegionOpts...)
		if err != nil {
			return nil, err
		}

		// Explicitly associate the attachment with your custom route table
		_, err = ec2transitgateway.NewRouteTableAssociation(ctx, fmt.Sprintf("%s-rt-assoc-%d", region, i), &ec2transitgateway.RouteTableAssociationArgs{
			TransitGatewayAttachmentId: attachment.ID(),
			TransitGatewayRouteTableId: tgwRt.ID(),
		}, opts...)
		if err != nil {
			return nil, err
		}

		// Explicitly propagate the VPC CIDR into the TGW route table
		_, err = ec2transitgateway.NewRouteTablePropagation(ctx, fmt.Sprintf("%s-rt-prop-%d", region, i), &ec2transitgateway.RouteTablePropagationArgs{
			TransitGatewayAttachmentId: attachment.ID(),
			TransitGatewayRouteTableId: tgwRt.ID(),
		}, opts...)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

func CreateTGWPeeringAttachmentAndRoutes(
	ctx *pulumi.Context,
	name string,
	localTgw *ec2transitgateway.TransitGateway,
	peerTgw *ec2transitgateway.TransitGateway,
	peerRegion string,
	eastCidr string,
	westCidr string,
	eastTgwRt *ec2transitgateway.RouteTable,
	westTgwRt *ec2transitgateway.RouteTable,
	optsLocal pulumi.ResourceOption,
	optsPeer pulumi.ResourceOption,
) (*ec2transitgateway.PeeringAttachment, *ec2transitgateway.PeeringAttachmentAccepter, error) {
	// Create the peering attachment in the local region
	peering, err := ec2transitgateway.NewPeeringAttachment(ctx, name+peerRegion, &ec2transitgateway.PeeringAttachmentArgs{
		TransitGatewayId:     localTgw.ID(),
		PeerTransitGatewayId: peerTgw.ID(),
		PeerRegion:           pulumi.String(peerRegion),
		Tags: pulumi.StringMap{
			"Name": pulumi.String(name + peerRegion),
		},
	}, pulumi.DependsOn([]pulumi.Resource{peerTgw}), optsLocal)
	if err != nil {
		return nil, nil, err
	}

	// Accept the peering attachment in the peer region, with DependsOn
	accepter, err := ec2transitgateway.NewPeeringAttachmentAccepter(ctx, name+"-accepter", &ec2transitgateway.PeeringAttachmentAccepterArgs{
		TransitGatewayAttachmentId: peering.ID(),
		Tags: pulumi.StringMap{
			"Name": pulumi.String(name + "-accepter"),
		},
	}, pulumi.DependsOn([]pulumi.Resource{peering}), optsPeer)
	if err != nil {
		return nil, nil, err
	}

	// Associate peering with custom route table
	_, err = ec2transitgateway.NewRouteTableAssociation(ctx, name+"-peering-rt-assoc", &ec2transitgateway.RouteTableAssociationArgs{
		TransitGatewayAttachmentId: peering.ID(),
		TransitGatewayRouteTableId: eastTgwRt.ID(),
	}, pulumi.DependsOn([]pulumi.Resource{accepter}), optsLocal)
	if err != nil {
		return nil, nil, err
	}

	// Associate accepter with custom route table
	_, err = ec2transitgateway.NewRouteTableAssociation(ctx, name+"-accepter-rt-assoc", &ec2transitgateway.RouteTableAssociationArgs{
		TransitGatewayAttachmentId: accepter.ID(),
		TransitGatewayRouteTableId: westTgwRt.ID(),
	}, pulumi.DependsOn([]pulumi.Resource{accepter}), optsPeer)
	if err != nil {
		return nil, nil, err
	}

	// Add route to peer CIDR in local TGW route table via the peering attachment
	_, err = ec2transitgateway.NewRoute(ctx, name+"-local-tgw-route", &ec2transitgateway.RouteArgs{
		TransitGatewayRouteTableId: eastTgwRt.ID(),
		DestinationCidrBlock:       pulumi.String(westCidr),
		TransitGatewayAttachmentId: peering.ID(),
	}, pulumi.DependsOn([]pulumi.Resource{accepter}), optsLocal)
	if err != nil {
		return nil, nil, err
	}

	// Add route to local CIDR in peer TGW route table via the peering attachment
	_, err = ec2transitgateway.NewRoute(ctx, name+"-peer-tgw-route", &ec2transitgateway.RouteArgs{
		TransitGatewayRouteTableId: westTgwRt.ID(),
		DestinationCidrBlock:       pulumi.String(eastCidr),
		TransitGatewayAttachmentId: peering.ID(),
	}, pulumi.DependsOn([]pulumi.Resource{accepter}), optsPeer)
	if err != nil {
		return nil, nil, err
	}

	return peering, accepter, nil
}
