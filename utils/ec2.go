package utils

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type InstanceArgs struct {
	Name           string
	VpcId          pulumi.IDOutput
	SubnetId       pulumi.IDOutput
	CidrForIngress string
	AmiId          string
}

func CreateEC2WithICMPAccess(ctx *pulumi.Context, args InstanceArgs) (*ec2.Instance, error) {
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
		return nil, err
	}

	// IAM Role for SSM
	role, err := iam.NewRole(ctx, fmt.Sprintf("%s-ssm-role", args.Name), &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(`{
            "Version": "2012-10-17",
            "Statement": [{
                "Effect": "Allow",
                "Principal": {"Service": "ec2.amazonaws.com"},
                "Action": "sts:AssumeRole"
            }]
        }`),
	})
	if err != nil {
		return nil, err
	}

	// Attach AmazonSSMManagedInstanceCore policy
	_, err = iam.NewRolePolicyAttachment(ctx, fmt.Sprintf("%s-ssm-policy-attach", args.Name), &iam.RolePolicyAttachmentArgs{
		Role:      role.Name,
		PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"),
	})
	if err != nil {
		return nil, err
	}

	// Attach AmazonSSMManagedInstanceCore policy
	_, err = iam.NewRolePolicyAttachment(ctx, fmt.Sprintf("%s-cw-policy-attach", args.Name), &iam.RolePolicyAttachmentArgs{
		Role:      role.Name,
		PolicyArn: pulumi.String("arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"),
	})
	if err != nil {
		return nil, err
	}

	// Instance Profile
	profile, err := iam.NewInstanceProfile(ctx, fmt.Sprintf("%s-ssm-profile", args.Name), &iam.InstanceProfileArgs{
		Role: role.Name,
	})
	if err != nil {
		return nil, err
	}

	instance, err := ec2.NewInstance(ctx, args.Name, &ec2.InstanceArgs{
		Ami:                 pulumi.String(args.AmiId),
		InstanceType:        pulumi.String("t4g.micro"),
		SubnetId:            args.SubnetId,
		VpcSecurityGroupIds: pulumi.StringArray{sg.ID()},
		IamInstanceProfile:  profile.Name,
		Tags: pulumi.StringMap{
			"Name": pulumi.String(args.Name),
		},
	})
	return instance, err
}
