import "strconv"

#portI: uint16 & <=65535

#portS: S={
	string & =~"^[0-9]{4,5}$"
	_i:     strconv.Atoi(S)
	#valid: uint16 & <=65535 & _i
}

#port: #portI | #portS
//#port: >0 & <= 65535

#url: string 

#imageURL: string =~ "^\(#url):\(#port)/mirror/oc-mirror-metadata$"

kind:       "ImageSetConfiguration"
apiVersion: "mirror.openshift.io/v1alpha2"
storageConfig: registry: {
	imageURL: imageURL
	skipTLS:  bool
}
mirror: {
	platform: channels: [{
		name: "stable-4.16"
		type: "ocp"
	}]
	operators: [{
		catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.19"
		packages?: [{
			name: "serverless-operator"
			channels: [{name: "stable"}]
		}]
	}]
	additionalImages: [{name: "registry.redhat.io/ubi8/ubi:latest"}]
	helm: {}
}
