package utils

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2clientvpn"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type VPNResult struct {
	Endpoint          *ec2clientvpn.Endpoint
	SubnetAssociation *ec2clientvpn.NetworkAssociation
	SecurityGroup     *ec2.SecurityGroup
}

type VPNArgs struct {
	VpcId                pulumi.IDOutput
	PrivateComputeSubnet pulumi.IDOutput
	DNS                  pulumi.StringOutput
	ClientCidrBlock      string
	ServerCertificateArn string
	SamlProviderArn      string
	SelfServiceSamlArn   string
}

func AddClientVPNRoute(
	ctx *pulumi.Context,
	name string,
	vpn *VPNResult, // Pass the VPNResult object
	targetNetworkCidr string,
	targetVpcSubnetId pulumi.IDOutput,
	opts ...pulumi.ResourceOption,
) (*ec2clientvpn.Route, error) {
	route, err := ec2clientvpn.NewRoute(ctx, name, &ec2clientvpn.RouteArgs{
		ClientVpnEndpointId:  vpn.Endpoint.ID(),
		DestinationCidrBlock: pulumi.String(targetNetworkCidr),
		TargetVpcSubnetId:    targetVpcSubnetId,
	}, append([]pulumi.ResourceOption{pulumi.DependsOn([]pulumi.Resource{vpn.SubnetAssociation})}, opts...)...)
	if err != nil {
		return nil, err
	}
	return route, nil
}

func CreateClientVPN(ctx *pulumi.Context, args VPNArgs, opts ...pulumi.ResourceOption) (*VPNResult, error) {
	logGroup, err := cloudwatch.NewLogGroup(ctx, "vpnLogGroup", &cloudwatch.LogGroupArgs{
		RetentionInDays: pulumi.Int(7),
	})
	if err != nil {
		return nil, err
	}

	vpnSG, err := ec2.NewSecurityGroup(ctx, "vpnSG", &ec2.SecurityGroupArgs{
		VpcId: args.VpcId,
		Ingress: ec2.SecurityGroupIngressArray{
			&ec2.SecurityGroupIngressArgs{
				Protocol:   pulumi.String("-1"),
				FromPort:   pulumi.Int(0),
				ToPort:     pulumi.Int(0),
				CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
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

	vpnEndpoint, err := ec2clientvpn.NewEndpoint(ctx, "vpnEndpoint", &ec2clientvpn.EndpointArgs{
		VpcId:                args.VpcId,
		SecurityGroupIds:     pulumi.StringArray{vpnSG.ID()},
		ClientCidrBlock:      pulumi.String(args.ClientCidrBlock),
		DnsServers:           pulumi.StringArray{args.DNS},
		ServerCertificateArn: pulumi.String(args.ServerCertificateArn),
		ConnectionLogOptions: &ec2clientvpn.EndpointConnectionLogOptionsArgs{
			Enabled:            pulumi.Bool(true),
			CloudwatchLogGroup: logGroup.Name,
		},
		AuthenticationOptions: ec2clientvpn.EndpointAuthenticationOptionArray{
			&ec2clientvpn.EndpointAuthenticationOptionArgs{
				Type:                       pulumi.String("federated-authentication"),
				SamlProviderArn:            pulumi.String(args.SamlProviderArn),
				SelfServiceSamlProviderArn: pulumi.String(args.SelfServiceSamlArn),
			},
		},
		SplitTunnel: pulumi.Bool(true),
		Tags: pulumi.StringMap{
			"Name": pulumi.String("ClientVPN"),
		},
	})
	if err != nil {
		return nil, err
	}

	subnetAssoc, err := ec2clientvpn.NewNetworkAssociation(ctx, "vpnAssoc", &ec2clientvpn.NetworkAssociationArgs{
		ClientVpnEndpointId: vpnEndpoint.ID(),
		SubnetId:            args.PrivateComputeSubnet,
	})
	if err != nil {
		return nil, err
	}

	_, err = ec2clientvpn.NewAuthorizationRule(ctx, "vpnAuthRule", &ec2clientvpn.AuthorizationRuleArgs{
		ClientVpnEndpointId: vpnEndpoint.ID(),
		TargetNetworkCidr: args.PrivateComputeSubnet.ApplyT(func(_ string) string {
			return "0.0.0.0/0"
		}).(pulumi.StringOutput),
		AuthorizeAllGroups: pulumi.Bool(true),
	})
	if err != nil {
		return nil, err
	}

	return &VPNResult{
		Endpoint:          vpnEndpoint,
		SubnetAssociation: subnetAssoc,
		SecurityGroup:     vpnSG,
	}, nil
}
