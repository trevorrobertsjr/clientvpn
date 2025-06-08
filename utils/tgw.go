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

func CreateTransitGateway(ctx *pulumi.Context, name string, opts ...pulumi.ResourceOption) (*ec2transitgateway.TransitGateway, error) {
	return ec2transitgateway.NewTransitGateway(ctx, name, &ec2transitgateway.TransitGatewayArgs{
		AmazonSideAsn:                pulumi.Int(64512),
		AutoAcceptSharedAttachments:  pulumi.String("enable"),
		DefaultRouteTableAssociation: pulumi.String("enable"),
		DefaultRouteTablePropagation: pulumi.String("enable"),
		Tags: pulumi.StringMap{
			"Name": pulumi.String(name),
		},
	}, opts...)
}

func AttachVPCsToTGW(
	ctx *pulumi.Context,
	tgw *ec2transitgateway.TransitGateway,
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
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-attachment-%d", region, i)),
			},
		}, dependsOnAndRegionOpts...)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

func CreateTGWPeeringAttachment(
	ctx *pulumi.Context,
	name string,
	localTgw *ec2transitgateway.TransitGateway,
	peerTgw *ec2transitgateway.TransitGateway,
	peerRegion string,
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

	return peering, accepter, nil
}
