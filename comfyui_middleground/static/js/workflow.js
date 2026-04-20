let currentPage = 1;
let pageSize = 20;

async function loadWorkflows() {
    const category = document.getElementById('categoryFilter').value;
    const search = document.getElementById('searchInput').value;

    const params = new URLSearchParams({
        page: currentPage,
        page_size: pageSize,
    });
    if (category) params.append('category', category);
    if (search) params.append('search', search);

    try {
        const response = await fetch(`/api/workflows?${params}`);
        const result = await response.json();

        if (result.code === 0) {
            renderWorkflows(result.data.workflows);
            updatePagination(result.data.total, result.data.page);
        }
    } catch (error) {
        console.error('加载工作流列表失败:', error);
        alert('加载工作流列表失败: ' + error.message);
    }
}

function renderWorkflows(workflows) {
    const tbody = document.getElementById('workflowTableBody');
    tbody.innerHTML = '';

    workflows.forEach(workflow => {
        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td>${workflow.id}</td>
            <td>${workflow.name}</td>
            <td>${workflow.description ? workflow.description.substring(0, 50) + '...' : '-'}</td>
            <td>${workflow.category || '-'}</td>
            <td>${workflow.tags || '-'}</td>
            <td>${workflow.usage_count}</td>
            <td>${new Date(workflow.created_at).toLocaleString()}</td>
            <td>
                <button onclick="showWorkflowDetail(${workflow.id})">查看</button>
                <button onclick="editWorkflow(${workflow.id})">编辑</button>
                <button onclick="executeWorkflow(${workflow.id})">执行</button>
                <button onclick="deleteWorkflow(${workflow.id})" class="btn-danger">删除</button>
            </td>
        `;
        tbody.appendChild(tr);
    });
}

function updatePagination(total, page) {
    const totalPages = Math.ceil(total / pageSize);
    document.getElementById('pageInfo').textContent = `第${page}页，共${totalPages}页`;
    currentPage = page;
}

function prevPage() {
    if (currentPage > 1) {
        currentPage--;
        loadWorkflows();
    }
}

function nextPage() {
    currentPage++;
    loadWorkflows();
}

function refreshWorkflows() {
    loadWorkflows();
}

function showCreateWorkflowModal() {
    document.getElementById('modalTitle').textContent = '新建工作流';
    document.getElementById('workflowForm').reset();
    document.getElementById('workflowId').value = '';
    document.getElementById('workflowModal').style.display = 'block';
}

function editWorkflow(id) {
    fetch(`/api/workflows/${id}`)
        .then(res => res.json())
        .then(result => {
            if (result.code === 0) {
                const workflow = result.data;
                document.getElementById('modalTitle').textContent = '编辑工作流';
                document.getElementById('workflowId').value = workflow.id;
                document.getElementById('workflowName').value = workflow.name;
                document.getElementById('workflowDescription').value = workflow.description || '';
                document.getElementById('workflowCategory').value = workflow.category || '';
                document.getElementById('workflowTags').value = workflow.tags || '';
                document.getElementById('workflowIsDefault').checked = workflow.is_default;
                document.getElementById('workflowIsPublic').checked = workflow.is_public;
                document.getElementById('workflowJSON').value = formatJSONString(workflow.workflow_json);
                // 加载提示词节点配置
                document.getElementById('positiveNodeID').value = workflow.positive_node_id || '';
                document.getElementById('positiveFieldName').value = workflow.positive_field_name || 'text';
                document.getElementById('negativeNodeID').value = workflow.negative_node_id || '';
                document.getElementById('negativeFieldName').value = workflow.negative_field_name || 'text';
                document.getElementById('workflowModal').style.display = 'block';
            }
        })
        .catch(error => {
            console.error('获取工作流详情失败:', error);
            alert('获取工作流详情失败: ' + error.message);
        });
}

function closeWorkflowModal() {
    document.getElementById('workflowModal').style.display = 'none';
}

async function showWorkflowDetail(id) {
    try {
        const response = await fetch(`/api/workflows/${id}`);
        const result = await response.json();

        if (result.code === 0) {
            const workflow = result.data;
            const modal = document.getElementById('workflowDetailModal');
            const content = document.getElementById('workflowDetailContent');

            content.innerHTML = `
                <p><strong>ID:</strong> ${workflow.id}</p>
                <p><strong>名称:</strong> ${workflow.name}</p>
                <p><strong>描述:</strong> ${workflow.description || '-'}</p>
                <p><strong>分类:</strong> ${workflow.category || '-'}</p>
                <p><strong>标签:</strong> ${workflow.tags || '-'}</p>
                <p><strong>使用次数:</strong> ${workflow.usage_count}</p>
                <p><strong>默认工作流:</strong> ${workflow.is_default ? '是' : '否'}</p>
                <p><strong>公开:</strong> ${workflow.is_public ? '是' : '否'}</p>
                <p><strong>创建者:</strong> ${workflow.created_by || '-'}</p>
                <p><strong>创建时间:</strong> ${new Date(workflow.created_at).toLocaleString()}</p>
                <p><strong>更新时间:</strong> ${new Date(workflow.updated_at).toLocaleString()}</p>
                <h3>工作流JSON:</h3>
                <pre class="json-preview">${formatJSONString(workflow.workflow_json)}</pre>
                <div class="modal-actions">
                    <button onclick="executeWorkflow(${workflow.id})">执行工作流</button>
                    <button onclick="editWorkflow(${workflow.id})">编辑</button>
                </div>
            `;

            modal.style.display = 'block';
        }
    } catch (error) {
        console.error('获取工作流详情失败:', error);
        alert('获取工作流详情失败: ' + error.message);
    }
}

function closeWorkflowDetail() {
    document.getElementById('workflowDetailModal').style.display = 'none';
}

async function deleteWorkflow(id) {
    if (!confirm('确定要删除这个工作流吗？')) {
        return;
    }

    try {
        const response = await fetch(`/api/workflows/${id}`, {
            method: 'DELETE',
        });
        const result = await response.json();

        if (result.code === 0) {
            alert('删除成功');
            loadWorkflows();
        } else {
            alert('删除失败: ' + result.message);
        }
    } catch (error) {
        console.error('删除工作流失败:', error);
        alert('删除失败: ' + error.message);
    }
}

async function executeWorkflow(id) {
    if (!confirm('确定要执行这个工作流吗？')) {
        return;
    }

    try {
        const response = await fetch(`/api/workflows/${id}/execute`, {
            method: 'POST',
        });
        const result = await response.json();

        if (result.code === 0) {
            alert(`工作流执行成功！\nPrompt ID: ${result.data.prompt_id}`);
            loadWorkflows(); // 刷新列表以更新使用次数
        } else {
            alert('执行失败: ' + result.message);
        }
    } catch (error) {
        console.error('执行工作流失败:', error);
        alert('执行失败: ' + error.message);
    }
}

// 自动检测提示词节点
function autoDetectPromptNodes() {
    const workflowJson = document.getElementById('workflowJSON').value;
    if (!workflowJson.trim()) {
        alert('请先输入工作流JSON');
        return;
    }

    try {
        const workflow = JSON.parse(workflowJson);
        let positiveFound = false;
        let negativeFound = false;

        // 优先查找包含 "positive" 字段的节点
        for (const [nodeId, node] of Object.entries(workflow)) {
            if (node.inputs) {
                // 查找包含 positive 字段的节点
                if (node.inputs.positive && typeof node.inputs.positive === 'string') {
                    if (!positiveFound) {
                        document.getElementById('positiveNodeID').value = nodeId;
                        document.getElementById('positiveFieldName').value = 'positive';
                        positiveFound = true;
                    }
                }
                // 查找包含 negative 字段的节点
                if (node.inputs.negative && typeof node.inputs.negative === 'string') {
                    if (!negativeFound) {
                        document.getElementById('negativeNodeID').value = nodeId;
                        document.getElementById('negativeFieldName').value = 'negative';
                        negativeFound = true;
                    }
                }
            }
        }

        // 如果没有找到 positive/negative 字段，查找 CLIPTextEncode 节点的 text 字段
        if (!positiveFound || !negativeFound) {
            for (const [nodeId, node] of Object.entries(workflow)) {
                if (node.class_type === 'CLIPTextEncode' && node.inputs && node.inputs.text) {
                    if (!positiveFound) {
                        document.getElementById('positiveNodeID').value = nodeId;
                        document.getElementById('positiveFieldName').value = 'text';
                        positiveFound = true;
                    } else if (!negativeFound) {
                        document.getElementById('negativeNodeID').value = nodeId;
                        document.getElementById('negativeFieldName').value = 'text';
                        negativeFound = true;
                        break;
                    }
                }
            }
        }

        // 如果还是没找到，查找任何包含字符串字段的节点（可能是提示词）
        if (!positiveFound || !negativeFound) {
            for (const [nodeId, node] of Object.entries(workflow)) {
                if (node.inputs) {
                    for (const [fieldName, fieldValue] of Object.entries(node.inputs)) {
                        if (typeof fieldValue === 'string' && fieldValue.length > 10) {
                            // 可能是提示词（长度较长）
                            if (!positiveFound && (fieldName.includes('positive') || fieldName.includes('prompt'))) {
                                document.getElementById('positiveNodeID').value = nodeId;
                                document.getElementById('positiveFieldName').value = fieldName;
                                positiveFound = true;
                            } else if (!negativeFound && (fieldName.includes('negative') || fieldName.includes('neg'))) {
                                document.getElementById('negativeNodeID').value = nodeId;
                                document.getElementById('negativeFieldName').value = fieldName;
                                negativeFound = true;
                            }
                        }
                    }
                }
            }
        }

        if (positiveFound || negativeFound) {
            const found = [];
            if (positiveFound) found.push('正向');
            if (negativeFound) found.push('负向');
            alert(`已检测到${found.join('和')}提示词节点`);
        } else {
            alert('未找到提示词节点，请手动配置节点ID和字段名');
        }
    } catch (error) {
        alert('工作流JSON格式错误: ' + error.message);
    }
}

// 表单提交
document.getElementById('workflowForm').addEventListener('submit', async (e) => {
    e.preventDefault();

    const id = document.getElementById('workflowId').value;
    const workflowData = {
        name: document.getElementById('workflowName').value,
        description: document.getElementById('workflowDescription').value,
        workflow_json: document.getElementById('workflowJSON').value,
        category: document.getElementById('workflowCategory').value,
        tags: document.getElementById('workflowTags').value,
        positive_node_id: document.getElementById('positiveNodeID').value,
        positive_field_name: document.getElementById('positiveFieldName').value || 'text',
        negative_node_id: document.getElementById('negativeNodeID').value,
        negative_field_name: document.getElementById('negativeFieldName').value || 'text',
        is_default: document.getElementById('workflowIsDefault').checked,
        is_public: document.getElementById('workflowIsPublic').checked,
        created_by: 'admin', // 可以从配置或登录信息获取
    };

    // 验证JSON格式
    try {
        JSON.parse(workflowData.workflow_json);
    } catch (error) {
        alert('工作流JSON格式错误: ' + error.message);
        return;
    }

    try {
        const url = id ? `/api/workflows/${id}` : '/api/workflows';
        const method = id ? 'PUT' : 'POST';

        const response = await fetch(url, {
            method: method,
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(workflowData),
        });

        const result = await response.json();
        if (result.code === 0) {
            alert(id ? '更新成功' : '创建成功');
            closeWorkflowModal();
            loadWorkflows();
        } else {
            alert((id ? '更新' : '创建') + '失败: ' + result.message);
        }
    } catch (error) {
        console.error('保存工作流失败:', error);
        alert('保存失败: ' + error.message);
    }
});

function loadWorkflowFromFile() {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    input.onchange = (e) => {
        const file = e.target.files[0];
        if (file) {
            const reader = new FileReader();
            reader.onload = (e) => {
                document.getElementById('workflowJSON').value = formatJSONString(e.target.result);
            };
            reader.readAsText(file);
        }
    };
    input.click();
}

function formatWorkflowJSON() {
    const textarea = document.getElementById('workflowJSON');
    try {
        const obj = JSON.parse(textarea.value);
        textarea.value = formatJSONString(JSON.stringify(obj));
    } catch (error) {
        alert('JSON格式错误: ' + error.message);
    }
}

function formatJSONString(jsonStr) {
    try {
        const obj = JSON.parse(jsonStr);
        return JSON.stringify(obj, null, 2);
    } catch (e) {
        return jsonStr;
    }
}

// 页面加载时初始化
window.addEventListener('DOMContentLoaded', () => {
    loadWorkflows();
});

