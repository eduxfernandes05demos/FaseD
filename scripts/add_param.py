fp = '/home/fred/projects/98 - AI Summit/quake-spec2cloud/infra/main.bicep'
with open(fp) as f:
    lines = f.readlines()

# Insert after line 32 (0-indexed: 31) which is 'param gameWorkerImageTag string = ...'
insert_at = 32
new_lines = [
    "\n",
    "@description('Container image tag to deploy to the streaming-gateway.')\n",
    "param streamingGatewayImageTag string = 'latest'\n",
]
for i, line in enumerate(new_lines):
    lines.insert(insert_at + i, line)

with open(fp, 'w') as f:
    f.writelines(lines)
print('Done - inserted param')
