{{- /* Shared pod spec used by both the Pod (podman play kube) and the
        Deployment (real K8s) wrappers. `.Values.deployAs` switches the
        podman-only bit: a hostPath mount of the repo root. See pod.yaml /
        deployment.yaml. */ -}}
{{- define "okub-ee.podspec" -}}
containers:
  - name: okub-ee
    image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
    imagePullPolicy: {{ .Values.image.pullPolicy }}
    command: {{ toJson .Values.command }}
    volumeMounts:
      - name: workspace
        mountPath: {{ .Values.workspace.mountPath }}
volumes:
  {{- if eq .Values.deployAs "Pod" }}
  # Pod mode: mount the repo root via hostPath so the project survives
  # `play kube --replace`. `emptyDir` would be anonymous/ephemeral storage.
  - name: workspace
    hostPath:
      path: {{ .Values.workspace.hostPath }}
      type: Directory
  {{- else }}
  # Deployment mode: ephemeral emptyDir hides image content at the mount path.
  # Populate it with an init container, or customize this template for a populated PVC.
  - name: workspace
    emptyDir: {}
  {{- end }}
{{- end -}}
