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
	})
}

func AttachVPCsToTGW(ctx *pulumi.Context, tgw *ec2transitgateway.TransitGateway, vpcs ...*VPCResult) ([]*ec2transitgateway.VpcAttachment, error) {
	var attachments []*ec2transitgateway.VpcAttachment
	for i, vpc := range vpcs {
		var subnetIds pulumi.StringArray
		for _, s := range vpc.PrivateTgwSubnets {
			subnetIds = append(subnetIds, s.ID())
		}
		attachment, err := ec2transitgateway.NewVpcAttachment(ctx, fmt.Sprintf("vpc-tgw-attachment-%d", i), &ec2transitgateway.VpcAttachmentArgs{
			VpcId:            vpc.Vpc.ID(),
			TransitGatewayId: tgw.ID(),
			SubnetIds:        subnetIds,
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("attachment-%d", i)),
			},
		})
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}
