let workflows = [];
let selectedWorkflow = null;
let currentRequestID = null;

// 页面加载时初始化
window.addEventListener('DOMContentLoaded', () => {
    loadWorkflowList();
});

// 加载工作流列表
async function loadWorkflowList() {
    try {
        const response = await fetch('/api/workflows?page=1&page_size=100');
        const result = await response.json();

        if (result.code === 0) {
            workflows = result.data.workflows;
            const select = document.getElementById('workflowSelect');
            select.innerHTML = '<option value="">请选择工作流...</option>';
            
            workflows.forEach(workflow => {
                const option = document.createElement('option');
                option.value = workflow.id;
                option.textContent = `${workflow.name}${workflow.is_default ? ' (默认)' : ''}`;
                select.appendChild(option);
            });
        }
    } catch (error) {
        console.error('加载工作流列表失败:', error);
        alert('加载工作流列表失败: ' + error.message);
    }
}

function refreshWorkflowList() {
    loadWorkflowList();
}

// 工作流选择变化
async function onWorkflowSelected() {
    const select = document.getElementById('workflowSelect');
    const workflowId = select.value;

    if (!workflowId) {
        selectedWorkflow = null;
        document.getElementById('workflowJson').value = '';
        return;
    }

    try {
        const response = await fetch(`/api/workflows/${workflowId}`);
        const result = await response.json();

        if (result.code === 0) {
            selectedWorkflow = result.data;
            document.getElementById('workflowJson').value = formatJSONString(selectedWorkflow.workflow_json);
            
            // 提取并显示原有的提示词（可选）
            const workflow = JSON.parse(selectedWorkflow.workflow_json);
            const workflowConfig = {
                positive_node_id: selectedWorkflow.positive_node_id,
                positive_field_name: selectedWorkflow.positive_field_name,
                negative_node_id: selectedWorkflow.negative_node_id,
                negative_field_name: selectedWorkflow.negative_field_name,
            };
            const prompts = extractPromptsFromWorkflow(workflow, workflowConfig);
            if (prompts.positive && !document.getElementById('testPositivePrompt').value) {
                document.getElementById('testPositivePrompt').placeholder = `原提示词: ${prompts.positive.substring(0, 50)}...`;
            }
            if (prompts.negative && !document.getElementById('testNegativePrompt').value) {
                document.getElementById('testNegativePrompt').placeholder = `原提示词: ${prompts.negative.substring(0, 50)}...`;
            }
        }
    } catch (error) {
        console.error('加载工作流详情失败:', error);
        alert('加载工作流详情失败: ' + error.message);
    }
}

// 从工作流中提取提示词（使用配置的节点和字段）
function extractPromptsFromWorkflow(workflow, workflowConfig) {
    let positivePrompt = '';
    let negativePrompt = '';

    // 如果配置了节点，使用配置的节点提取
    if (workflowConfig && workflowConfig.positive_node_id) {
        const node = workflow[workflowConfig.positive_node_id];
        if (node && node.inputs) {
            const fieldName = workflowConfig.positive_field_name || 'text';
            positivePrompt = node.inputs[fieldName] || '';
        }
    }

    if (workflowConfig && workflowConfig.negative_node_id) {
        const node = workflow[workflowConfig.negative_node_id];
        if (node && node.inputs) {
            const fieldName = workflowConfig.negative_field_name || 'text';
            negativePrompt = node.inputs[fieldName] || '';
        }
    }

    // 如果没有配置，回退到自动检测
    if (!positivePrompt && !negativePrompt) {
        for (const [nodeId, node] of Object.entries(workflow)) {
            if (node.class_type === 'CLIPTextEncode') {
                if (node.inputs && node.inputs.text) {
                    if (!positivePrompt) {
                        positivePrompt = node.inputs.text;
                    } else if (!negativePrompt) {
                        negativePrompt = node.inputs.text;
                        break;
                    }
                }
            }
        }
    }

    return { positive: positivePrompt, negative: negativePrompt };
}

// 替换工作流中的提示词（使用配置的节点和字段）
function replacePromptsInWorkflow(workflow, positivePrompt, negativePrompt, workflowConfig) {
    const workflowCopy = JSON.parse(JSON.stringify(workflow)); // 深拷贝

    // 如果工作流配置了节点和字段，使用配置
    if (workflowConfig && workflowConfig.positive_node_id && positivePrompt) {
        const positiveNode = workflowCopy[workflowConfig.positive_node_id];
        if (positiveNode && positiveNode.inputs) {
            const fieldName = workflowConfig.positive_field_name || 'text';
            positiveNode.inputs[fieldName] = positivePrompt;
            console.log(`替换正向提示词: 节点ID=${workflowConfig.positive_node_id}, 字段名=${fieldName}`);
        } else {
            console.warn(`警告: 未找到节点 ${workflowConfig.positive_node_id} 或节点没有 inputs 字段`);
        }
    }

    if (workflowConfig && workflowConfig.negative_node_id && negativePrompt) {
        const negativeNode = workflowCopy[workflowConfig.negative_node_id];
        if (negativeNode && negativeNode.inputs) {
            const fieldName = workflowConfig.negative_field_name || 'text';
            negativeNode.inputs[fieldName] = negativePrompt;
            console.log(`替换负向提示词: 节点ID=${workflowConfig.negative_node_id}, 字段名=${fieldName}`);
        } else {
            console.warn(`警告: 未找到节点 ${workflowConfig.negative_node_id} 或节点没有 inputs 字段`);
        }
    }

    return workflowCopy;
}

// 自动查找并替换工作流中的提示词（递归查找 positive/negative 字段）
function replacePromptsInWorkflowAuto(workflow, positivePrompt, negativePrompt) {
    const workflowCopy = JSON.parse(JSON.stringify(workflow)); // 深拷贝

    // 递归查找并替换函数
    function replaceField(obj, fieldName, value) {
        if (typeof obj !== 'object' || obj === null) {
            return false;
        }

        // 如果是数组，递归查找每个元素
        if (Array.isArray(obj)) {
            for (const item of obj) {
                if (replaceField(item, fieldName, value)) {
                    return true;
                }
            }
            return false;
        }

        // 如果是对象
        for (const key in obj) {
            if (key === fieldName && typeof obj[key] === 'string' && obj[key].length > 0) {
                obj[key] = value;
                console.log(`自动替换提示词: 字段名=${fieldName}`);
                return true;
            }
            // 递归查找子对象
            if (typeof obj[key] === 'object' && obj[key] !== null) {
                if (replaceField(obj[key], fieldName, value)) {
                    return true;
                }
            }
        }
        return false;
    }

    // 替换正向提示词
    if (positivePrompt) {
        // 优先查找 positive 字段
        if (!replaceField(workflowCopy, 'positive', positivePrompt)) {
            // 如果没找到 positive，查找 CLIPTextEncode 节点的 text 字段
            console.log('未找到 positive 字段，尝试查找 CLIPTextEncode 节点的 text 字段');
            for (const [nodeId, node] of Object.entries(workflowCopy)) {
                if (node.class_type === 'CLIPTextEncode' && node.inputs && node.inputs.text) {
                    node.inputs.text = positivePrompt;
                    console.log(`自动替换正向提示词: 节点ID=${nodeId}, 字段名=text`);
                    break;
                }
            }
        }
    }

    // 替换负向提示词
    if (negativePrompt) {
        // 优先查找 negative 字段
        if (!replaceField(workflowCopy, 'negative', negativePrompt)) {
            // 如果没找到 negative，查找第二个 CLIPTextEncode 节点
            console.log('未找到 negative 字段，尝试查找 CLIPTextEncode 节点的 text 字段');
            let foundFirst = false;
            for (const [nodeId, node] of Object.entries(workflowCopy)) {
                if (node.class_type === 'CLIPTextEncode' && node.inputs && node.inputs.text) {
                    if (foundFirst) {
                        node.inputs.text = negativePrompt;
                        console.log(`自动替换负向提示词: 节点ID=${nodeId}, 字段名=text`);
                        break;
                    }
                    foundFirst = true;
                }
            }
        }
    }

    return workflowCopy;
}

async function submitTestRequest() {
    const workflowSelect = document.getElementById('workflowSelect');
    const workflowId = workflowSelect.value;

    if (!workflowId) {
        alert('请先选择工作流');
        return;
    }

    if (!selectedWorkflow) {
        alert('工作流数据未加载，请重新选择');
        return;
    }

    const positivePrompt = document.getElementById('testPositivePrompt').value.trim();
    const negativePrompt = document.getElementById('testNegativePrompt').value.trim();

    // 解析工作流JSON
    let workflow;
    try {
        workflow = JSON.parse(selectedWorkflow.workflow_json);
    } catch (e) {
        alert('工作流JSON格式错误: ' + e.message);
        return
    }

    // 如果输入了提示词，根据配置的节点和字段替换工作流中的提示词
    // 如果没有配置，自动查找 positive/negative 字段
    if (positivePrompt || negativePrompt) {
        const workflowConfig = {
            positive_node_id: selectedWorkflow.positive_node_id,
            positive_field_name: selectedWorkflow.positive_field_name,
            negative_node_id: selectedWorkflow.negative_node_id,
            negative_field_name: selectedWorkflow.negative_field_name,
        };
        
        // 如果配置了节点，使用配置；否则自动查找
        if (workflowConfig.positive_node_id || workflowConfig.negative_node_id) {
            workflow = replacePromptsInWorkflow(workflow, positivePrompt, negativePrompt, workflowConfig);
        } else {
            workflow = replacePromptsInWorkflowAuto(workflow, positivePrompt, negativePrompt);
        }
    }

    try {
        const response = await fetch('/api/test/request', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                workflow_id: parseInt(workflowId),
                prompt_text: positivePrompt,
                negative_prompt: negativePrompt,
                workflow: workflow,
            }),
        });

        const result = await response.json();
        if (result.code === 0) {
            showTestResult(result.data);
        } else {
            alert('提交失败: ' + result.message);
        }
    } catch (error) {
        console.error('提交测试请求失败:', error);
        alert('提交失败: ' + error.message);
    }
}

function showTestResult(data) {
    const resultDiv = document.getElementById('testResult');
    const contentDiv = document.getElementById('testResultContent');

    currentRequestID = data.request_id;

    contentDiv.innerHTML = `
        <p><strong>请求ID:</strong> ${data.request_id}</p>
        <p><strong>Prompt ID:</strong> ${data.prompt_id}</p>
        <p><strong>状态:</strong> ${data.status}</p>
        <button onclick="refreshTaskStatus()" style="margin-top: 10px; padding: 5px 15px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer;">刷新状态</button>
    `;

    resultDiv.style.display = 'block';
}

// 手动刷新任务状态
async function refreshTaskStatus() {
    if (!currentRequestID) {
        alert('没有当前任务');
        return;
    }

    try {
        const response = await fetch(`/api/tasks/${currentRequestID}`);
        const result = await response.json();

        if (result.code === 0) {
            const task = result.data;
            const contentDiv = document.getElementById('testResultContent');
            
            let statusHTML = `
                <p><strong>请求ID:</strong> ${task.request_id}</p>
                <p><strong>Prompt ID:</strong> ${task.prompt_id || '-'}</p>
                <p><strong>状态:</strong> ${task.status}</p>
                <p><strong>进度:</strong> ${task.progress}%</p>
            `;

            // 如果任务已完成或失败，显示图像
            if (task.status === 'completed' && task.files_info) {
                try {
                    const files = JSON.parse(task.files_info);
                    if (files.length > 0) {
                        statusHTML += '<h4>生成的图像:</h4>';
                        files.forEach(file => {
                            statusHTML += `<img src="${file.url}" alt="${file.filename}" style="max-width: 300px; margin: 10px;">`;
                        });
                    }
                } catch (e) {
                    console.error('解析文件信息失败:', e);
                }
            }

            // 如果任务未完成，显示刷新按钮
            if (task.status !== 'completed' && task.status !== 'failed') {
                statusHTML += '<button onclick="refreshTaskStatus()" style="margin-top: 10px; padding: 5px 15px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer;">刷新状态</button>';
            }

            contentDiv.innerHTML = statusHTML;
        } else {
            alert('查询任务状态失败: ' + result.message);
        }
    } catch (error) {
        console.error('查询任务状态失败:', error);
        alert('查询任务状态失败: ' + error.message);
    }
}

function clearTestForm() {
    document.getElementById('workflowSelect').value = '';
    document.getElementById('testPositivePrompt').value = '';
    document.getElementById('testNegativePrompt').value = '';
    document.getElementById('workflowJson').value = '';
    document.getElementById('testResult').style.display = 'none';
    selectedWorkflow = null;
    currentRequestID = null;
}

function loadWorkflowFile() {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    input.onchange = (e) => {
        const file = e.target.files[0];
        if (file) {
            const reader = new FileReader();
            reader.onload = (e) => {
                document.getElementById('workflowJson').value = formatJSONString(e.target.result);
                document.getElementById('workflowSelect').value = '';
                selectedWorkflow = null;
            };
            reader.readAsText(file);
        }
    };
    input.click();
}

function formatWorkflowJSON() {
    const textarea = document.getElementById('workflowJson');
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
