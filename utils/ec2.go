package utils

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type InstanceArgs struct {
	Name           string
	VpcId          pulumi.IDOutput
	SubnetId       pulumi.IDOutput
	CidrForIngress string
	AmiId          string
}

func CreateEC2WithICMPAccess(ctx *pulumi.Context, args InstanceArgs) error {
	// Security Group allowing ICMP
	sg, err := ec2.NewSecurityGroup(ctx, fmt.Sprintf("%s-sg", args.Name), &ec2.SecurityGroupArgs{
		VpcId: args.VpcId,
		Ingress: ec2.SecurityGroupIngressArray{
			&ec2.SecurityGroupIngressArgs{
				Protocol:   pulumi.String("icmp"),
				FromPort:   pulumi.Int(-1),
				ToPort:     pulumi.Int(-1),
				CidrBlocks: pulumi.StringArray{pulumi.String(args.CidrForIngress)},
			},
		},
		Egress: ec2.SecurityGroupEgressArray{
			&ec2.SecurityGroupEgressArgs{
				Protocol:   pulumi.String("-1"),
				FromPort:   pulumi.Int(0),
				ToPort:     pulumi.Int(0),
				CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			},
		},
	})
	if err != nil {
		return err
	}

	_, err = ec2.NewInstance(ctx, args.Name, &ec2.InstanceArgs{
		Ami:                 pulumi.String(args.AmiId),
		InstanceType:        pulumi.String("t4g.micro"),
		SubnetId:            args.SubnetId,
		VpcSecurityGroupIds: pulumi.StringArray{sg.ID()},
		Tags: pulumi.StringMap{
			"Name": pulumi.String(args.Name),
		},
	})
	return err
}
