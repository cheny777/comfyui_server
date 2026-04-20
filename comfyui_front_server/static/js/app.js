// API基础配置
const API_BASE = '/api';
let token = null;
let currentTaskId = null;
let workflows = []; // 工作流列表
let selectedWorkflow = null; // 当前选中的工作流
let progressPollingInterval = null; // 任务进度轮询定时器

// 生成或获取设备ID
function getOrCreateDeviceID() {
    let deviceID = localStorage.getItem('device_id');
    if (!deviceID) {
        // 生成UUID v4
        deviceID = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
            const r = Math.random() * 16 | 0;
            const v = c === 'x' ? r : (r & 0x3 | 0x8);
            return v.toString(16);
        });
        localStorage.setItem('device_id', deviceID);
    }
    return deviceID;
}

// 用户初始化流程
async function initUser() {
    const deviceID = getOrCreateDeviceID();
    const response = await fetch(`${API_BASE}/user/init`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            device_id: deviceID,
            nickname: ''
        })
    });
    
    const result = await response.json();
    if (result.code === 0) {
        // 存储Token
        token = result.data.token;
        localStorage.setItem('token', token);
        localStorage.setItem('user_id', result.data.user.id);
        updateUserInfo(result.data.user);
        return result.data;
    }
    throw new Error(result.message);
}

// 获取Token
function getToken() {
    if (!token) {
        token = localStorage.getItem('token');
    }
    return token;
}

// 检查Token是否有效，无效则重新初始化
async function ensureUser() {
    const token = getToken();
    if (!token) {
        return await initUser();
    }
    
    // 验证Token是否有效
    try {
        const response = await fetch(`${API_BASE}/user/info`, {
            headers: {
                'Authorization': `Bearer ${token}`
            }
        });
        if (response.ok) {
            const result = await response.json();
            if (result.code === 0) {
                updateUserInfo(result.data);
                return result.data;
            }
        }
    } catch (e) {
        console.error('Token验证失败:', e);
    }
    
    return await initUser();
}

// 更新用户信息显示
function updateUserInfo(user) {
    const nicknameEl = document.getElementById('userNickname');
    if (nicknameEl) {
        nicknameEl.textContent = user.nickname || '游客';
    }
}

// WebSocket连接已移除，改用定时查询机制获取任务状态
// 任务进度通过 startTaskProgressPolling 定时查询获取

// 加载工作流列表
async function loadWorkflows() {
    try {
        const response = await fetch(`${API_BASE}/workflows?page=1&page_size=100`, {
            headers: {
                'Authorization': `Bearer ${getToken()}`
            }
        });
        const result = await response.json();
        
        if (result.code === 0) {
            workflows = result.data.workflows || [];
            renderWorkflowSelect();
        } else {
            console.error('加载工作流列表失败:', result.message);
            const select = document.getElementById('workflowSelect');
            if (select) {
                select.innerHTML = '<option value="">加载失败，请刷新页面</option>';
            }
        }
    } catch (error) {
        console.error('加载工作流列表失败:', error);
        const select = document.getElementById('workflowSelect');
        if (select) {
            select.innerHTML = '<option value="">加载失败，请刷新页面</option>';
        }
    }
}

// 渲染工作流选择下拉框
function renderWorkflowSelect() {
    const select = document.getElementById('workflowSelect');
    if (!select) return;
    
    if (workflows.length === 0) {
        select.innerHTML = '<option value="">暂无工作流</option>';
        select.disabled = true;
        return;
    }
    
    select.disabled = false;
    select.innerHTML = '<option value="">请选择工作流</option>' + 
        workflows.map(wf => {
            const name = wf.name || wf.id || '未命名工作流';
            return `<option value="${wf.id}">${name}</option>`;
        }).join('');
    
    // 监听选择变化
    select.onchange = function() {
        const workflowId = this.value;
        selectedWorkflow = workflows.find(wf => wf.id == workflowId) || null;
        
        // 如果工作流有默认提示词，自动填充到输入框
        if (selectedWorkflow) {
            console.log('已选择工作流:', selectedWorkflow.name);
            // 提取并填充提示词
            extractAndFillPrompts(selectedWorkflow);
        } else {
            console.log('未选择工作流');
            // 清空提示词输入框
            document.getElementById('positivePrompt').value = '';
        }
    };
}

// 从工作流中提取提示词并填充到输入框
function extractAndFillPrompts(workflow) {
    if (!workflow || !workflow.workflow_json) {
        return;
    }

    // 解析工作流JSON
    let workflowObj = workflow.workflow_json;
    if (typeof workflowObj === 'string') {
        try {
            workflowObj = JSON.parse(workflowObj);
        } catch (e) {
            console.error('解析工作流JSON失败:', e);
            return;
        }
    }

    // 提取正向提示词
    let positivePrompt = '';
    let negativePrompt = '';

    // 如果工作流配置了节点和字段，使用配置提取
    if (workflow.positive_node_id || workflow.negative_node_id) {
        const positiveNodeId = workflow.positive_node_id;
        const positiveFieldName = workflow.positive_field_name || 'text';
        const negativeNodeId = workflow.negative_node_id;
        const negativeFieldName = workflow.negative_field_name || 'text';

        // 提取正向提示词
        if (positiveNodeId && workflowObj[positiveNodeId]) {
            const node = workflowObj[positiveNodeId];
            if (node.inputs && node.inputs[positiveFieldName]) {
                positivePrompt = node.inputs[positiveFieldName] || '';
            }
        }

        // 提取负向提示词
        if (negativeNodeId && workflowObj[negativeNodeId]) {
            const node = workflowObj[negativeNodeId];
            if (node.inputs && node.inputs[negativeFieldName]) {
                negativePrompt = node.inputs[negativeFieldName] || '';
            }
        }
    } else {
        // 如果没有配置，自动查找 CLIPTextEncode 节点
        let positiveFound = false;
        for (const nodeId in workflowObj) {
            const node = workflowObj[nodeId];
            if (node && node.class_type === 'CLIPTextEncode') {
                if (node.inputs && node.inputs.text !== undefined) {
                    const text = node.inputs.text || '';
                    if (!positiveFound) {
                        positivePrompt = text;
                        positiveFound = true;
                    } else {
                        negativePrompt = text;
                        break;
                    }
                }
            }
        }
    }

    // 填充到输入框
    const positiveInput = document.getElementById('positivePrompt');
    
    if (positiveInput) {
        positiveInput.value = positivePrompt || '';
    }

    console.log('已提取提示词:', { positive: positivePrompt });
}

// 替换工作流中的提示词
function replacePromptsInWorkflow(workflow, positivePrompt, negativePrompt) {
    if (!workflow || typeof workflow !== 'object') {
        return workflow;
    }
    
    // 深拷贝工作流
    const newWorkflow = JSON.parse(JSON.stringify(workflow));
    
    // 如果工作流有配置的提示词节点信息，使用配置的节点
    if (selectedWorkflow) {
        // 注意：字段名是 positive_node_id, positive_field_name 等（不是 positive_prompt_node_id）
        const positiveNodeId = selectedWorkflow.positive_node_id;
        const positiveFieldName = selectedWorkflow.positive_field_name || 'text';
        const negativeNodeId = selectedWorkflow.negative_node_id;
        const negativeFieldName = selectedWorkflow.negative_field_name || 'text';
        
        // 替换正向提示词
        if (positiveNodeId && newWorkflow[positiveNodeId]) {
            if (newWorkflow[positiveNodeId].inputs) {
                newWorkflow[positiveNodeId].inputs[positiveFieldName] = positivePrompt || '';
            }
        }
        
        // 替换负向提示词
        if (negativeNodeId && newWorkflow[negativeNodeId]) {
            if (newWorkflow[negativeNodeId].inputs) {
                newWorkflow[negativeNodeId].inputs[negativeFieldName] = negativePrompt || '';
            }
        }
    } else {
        // 如果没有配置，尝试自动查找 CLIPTextEncode 节点
        let positiveFound = false;
        for (const nodeId in newWorkflow) {
            const node = newWorkflow[nodeId];
            if (node && node.class_type === 'CLIPTextEncode') {
                if (node.inputs && node.inputs.text !== undefined) {
                    // 第一个找到的节点作为正向提示词，第二个作为负向提示词
                    if (!positiveFound) {
                        node.inputs.text = positivePrompt || '';
                        positiveFound = true;
                    } else if (negativePrompt) {
                        node.inputs.text = negativePrompt;
                        break;
                    }
                }
            }
        }
    }
    
    return newWorkflow;
}

// 提交生成任务
async function submitTask() {
    const workflowSelect = document.getElementById('workflowSelect');
    const positivePrompt = document.getElementById('positivePrompt').value;
    const negativePrompt = ''; // 负向提示词已移除，设为空字符串
    
    // 验证工作流选择
    if (!workflowSelect || !workflowSelect.value) {
        alert('请先选择工作流');
        return;
    }
    
    if (!positivePrompt.trim()) {
        alert('请输入正向提示词');
        return;
    }
    
    if (!selectedWorkflow) {
        alert('工作流数据无效，请重新选择');
        return;
    }

    const generateBtn = document.getElementById('generateBtn');
    generateBtn.disabled = true;
    generateBtn.textContent = '生成中...';
    generateBtn.classList.add('loading');

    // 构建请求体：优先使用 workflow_id + 提示词的方式
    let requestBody;
    if (selectedWorkflow.id) {
        // 使用 workflow_id + 提示词的方式（推荐）
        requestBody = {
            workflow_id: selectedWorkflow.id,
            prompt_text: positivePrompt,
            negative_prompt: negativePrompt
        };
        console.log('使用 workflow_id 方式提交:', requestBody);
    } else if (selectedWorkflow.workflow_json) {
        // 向后兼容：如果没有 workflow_id，使用 workflow 方式
        let workflow = selectedWorkflow.workflow_json;
        if (typeof workflow === 'string') {
            try {
                workflow = JSON.parse(workflow);
            } catch (e) {
                console.error('解析工作流JSON失败:', e);
                alert('工作流格式错误');
                generateBtn.disabled = false;
                generateBtn.textContent = '生成图像';
                return;
            }
        }
        
        // 替换提示词
        workflow = replacePromptsInWorkflow(workflow, positivePrompt, negativePrompt);
        requestBody = { workflow };
        console.log('使用 workflow 方式提交');
    } else {
        alert('工作流数据无效，请重新选择');
        generateBtn.disabled = false;
        generateBtn.textContent = '生成图像';
        return;
    }
    
    try {
        const response = await fetch(`${API_BASE}/tasks`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${getToken()}`
            },
            body: JSON.stringify(requestBody)
        });
        
        // 检查HTTP状态码
        if (!response.ok) {
            const errorText = await response.text();
            console.error('HTTP错误:', response.status, errorText);
            showNotification(`提交任务失败: HTTP ${response.status}`, 'error');
            generateBtn.disabled = false;
            generateBtn.textContent = '生成图像';
            generateBtn.classList.remove('loading');
            return;
        }
        
        const result = await response.json();
        console.log('任务提交响应:', result);
        
        // 检查响应格式
        if (!result || typeof result !== 'object') {
            console.error('响应格式错误:', result);
            showNotification('提交任务失败: 响应格式错误', 'error');
            generateBtn.disabled = false;
            generateBtn.textContent = '生成图像';
            generateBtn.classList.remove('loading');
            return;
        }
        
        // 检查响应码
        if (result.code === 0 || result.code === undefined) {
            // 成功：code为0或未定义都视为成功
            const taskId = result.data?.task_id || result.task_id;
            const taskStatus = result.data?.status || result.status || 'pending';
            
            if (!taskId) {
                console.error('响应中缺少task_id:', result);
                showNotification('提交任务失败: 响应中缺少任务ID', 'error');
                generateBtn.disabled = false;
                generateBtn.textContent = '生成图像';
                generateBtn.classList.remove('loading');
                return;
            }
            
            currentTaskId = taskId;
            
            // 显示成功提示
            showNotification('任务提交成功！', 'success');
            
            // 清空图像网格，准备显示新图像
            clearImageGrid();
            
            // 显示进度
            showProgress();
            updateProgress(0, taskStatus);
            
            // 启动定时查询任务进度
            startTaskProgressPolling(currentTaskId);
            
            // 滚动到进度区域
            const progressSection = document.getElementById('progressSection');
            if (progressSection) {
                progressSection.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
            }
        } else {
            // 失败：code不为0
            const errorMsg = result.message || result.error || '未知错误';
            console.error('任务提交失败:', result);
            showNotification('提交任务失败: ' + errorMsg, 'error');
            generateBtn.disabled = false;
            generateBtn.textContent = '生成图像';
            generateBtn.classList.remove('loading');
        }
    } catch (error) {
        console.error('提交任务失败:', error);
        showNotification('提交任务失败: ' + (error.message || '网络错误'), 'error');
        generateBtn.disabled = false;
        generateBtn.textContent = '生成图像';
        generateBtn.classList.remove('loading');
    }
}

// 显示进度
function showProgress() {
    const progressSection = document.getElementById('progressSection');
    if (progressSection) {
        progressSection.style.display = 'block';
    }
}

// 启动任务进度定时查询
function startTaskProgressPolling(taskId) {
    // 停止之前的查询
    stopTaskProgressPolling();
    
    // 每2秒查询一次任务状态
    progressPollingInterval = setInterval(async () => {
        if (!taskId || taskId !== currentTaskId) {
            stopTaskProgressPolling();
            return;
        }
        
        try {
            const response = await fetch(`${API_BASE}/tasks/${taskId}`, {
                headers: {
                    'Authorization': `Bearer ${getToken()}`
                }
            });
            
            const result = await response.json();
            if (result.code === 0) {
                const task = result.data;
                const progress = task.progress || 0;
                const status = task.status || 'running';
                
                // 更新进度显示
                updateProgress(progress, status);
                
                // 如果任务已完成或失败，停止查询
                if (status === 'completed' || status === 'failed') {
                    stopTaskProgressPolling();
                    
                    if (status === 'completed') {
                        showNotification('图像生成完成！', 'success');
                        // 使用新接口查询图像文件列表
                        loadTaskImages(taskId);
                    } else if (status === 'failed') {
                        showNotification('任务执行失败: ' + (task.error || '未知错误'), 'error');
                    }
                    
                    loadHistory();
                }
            } else {
                console.error('查询任务状态失败:', result.message);
            }
        } catch (error) {
            console.error('查询任务状态失败:', error);
        }
    }, 2000); // 每2秒查询一次
}

// 停止任务进度定时查询
function stopTaskProgressPolling() {
    if (progressPollingInterval) {
        clearInterval(progressPollingInterval);
        progressPollingInterval = null;
    }
}

// 更新进度
function updateProgress(progress, status) {
    const progressBar = document.getElementById('progressBar');
    const progressText = document.getElementById('progressText');
    const progressSection = document.getElementById('progressSection');
    
    if (progressBar) {
        // 添加平滑过渡动画
        progressBar.style.transition = 'width 0.3s ease';
        progressBar.style.width = progress + '%';
        
        // 根据状态设置颜色
        if (status === 'completed') {
            progressBar.style.backgroundColor = '#4caf50';
        } else if (status === 'failed') {
            progressBar.style.backgroundColor = '#f44336';
        } else if (status === 'running') {
            progressBar.style.backgroundColor = '#2196f3';
        } else {
            progressBar.style.backgroundColor = '#ff9800';
        }
    }
    
    if (progressText) {
        const statusText = {
            'pending': '⏳ 等待中...',
            'running': `🎨 生成中... ${progress}%`,
            'completed': '✅ 已完成',
            'failed': '❌ 失败'
        };
        progressText.textContent = statusText[status] || `进度: ${progress}%`;
        
        // 添加状态类
        progressText.className = `progress-status ${status}`;
    }

    // 如果完成或失败，恢复按钮
    if (status === 'completed' || status === 'failed') {
        stopTaskProgressPolling();
        const generateBtn = document.getElementById('generateBtn');
        if (generateBtn) {
            generateBtn.disabled = false;
            generateBtn.textContent = '生成图像';
            generateBtn.classList.remove('loading');
        }
        
        // 隐藏进度条（可选）
        if (status === 'completed' && progressSection) {
            setTimeout(() => {
                progressSection.style.opacity = '0.5';
            }, 2000);
        }
    }
}

// 清空图像网格
function clearImageGrid() {
    const imageGrid = document.getElementById('imageGrid');
    if (imageGrid) {
        imageGrid.innerHTML = '';
    }
}

// 显示图像列表
function displayImages(images) {
    const imageGrid = document.getElementById('imageGrid');
    if (!imageGrid) return;

    // 清除空状态
    const emptyState = imageGrid.querySelector('.empty-state');
    if (emptyState) {
        emptyState.remove();
    }

    // 如果图像列表为空，显示空状态
    if (!images || images.length === 0) {
        imageGrid.innerHTML = '<div class="empty-state">暂无图像</div>';
        return;
    }

    // 添加图像到网格（添加动画效果）
    images.forEach((image, index) => {
        setTimeout(() => {
            addImageToGrid(image);
        }, index * 100); // 逐个显示，添加延迟动画
    });
}

// 添加图像到网格
function addImageToGrid(image) {
    const imageGrid = document.getElementById('imageGrid');
    if (!imageGrid) return;

    const imageCard = document.createElement('div');
    imageCard.className = 'image-card';
    imageCard.style.opacity = '0';
    imageCard.style.transform = 'scale(0.9)';
    imageCard.style.transition = 'all 0.3s ease';
    
    // 优先使用 data_url（如果中台返回了文件内容），否则使用 url
    // 支持多种数据格式：image.data_url, image.file?.data_url, image.url, image.file?.url
    const url = image.data_url || image.file?.data_url || image.url || image.file?.url || '';
    const filename = image.filename || image.file?.filename || 'image.png';
    const fileContent = image.file?.file_content || image.file_content || null;
    const mimeType = image.file?.mime_type || image.file?.file_type || image.file_type || 'image/png';
    
    console.log('添加图像到网格:', { filename, hasDataUrl: !!url, urlLength: url.length, hasFileContent: !!fileContent });
    
    // 转义文件名，防止XSS（URL如果是data URL可能很长，不需要转义）
    const safeFilename = filename.replace(/'/g, "\\'").replace(/"/g, '&quot;');
    
    // 对于data URL，直接使用；对于普通URL，需要转义
    let safeUrl;
    if (url.startsWith('data:')) {
        // data URL 可以直接使用，但需要转义单引号以避免HTML属性问题
        safeUrl = url.replace(/'/g, "\\'");
    } else {
        // 普通URL需要转义
        safeUrl = url.replace(/'/g, "\\'").replace(/"/g, '&quot;');
    }
    
    // 将文件内容转换为JSON字符串，用于下载函数
    const fileContentJson = fileContent ? JSON.stringify(fileContent).replace(/'/g, "\\'").replace(/"/g, '&quot;') : 'null';
    const mimeTypeSafe = mimeType.replace(/'/g, "\\'").replace(/"/g, '&quot;');
    
    // 使用 data-* 属性存储数据，避免在 onclick 中传递长字符串
    imageCard.innerHTML = `
        <div class="image-wrapper">
            <img src="${safeUrl}" alt="${safeFilename}" 
                 data-image-url="${safeUrl}"
                 data-image-filename="${safeFilename}"
                 data-image-content="${fileContentJson}"
                 data-image-mimetype="${mimeTypeSafe}"
                 onclick="previewImageFromCard(this)"
                 style="cursor: pointer;"
                 onload="this.parentElement.classList.add('loaded')"
                 onerror="this.src='data:image/svg+xml,%3Csvg xmlns=\'http://www.w3.org/2000/svg\' width=\'200\' height=\'200\'%3E%3Crect fill=\'%23ddd\' width=\'200\' height=\'200\'/%3E%3Ctext fill=\'%23999\' font-family=\'sans-serif\' font-size=\'14\' x=\'50%25\' y=\'50%25\' text-anchor=\'middle\' dy=\'.3em\'%3E加载失败%3C/text%3E%3C/svg%3E'; this.parentElement.classList.add('error')">
            <div class="image-overlay">
                <button class="preview-btn" onclick="previewImageFromCard(this.closest('.image-wrapper').querySelector('img'))" title="预览">
                    <span>👁</span> 预览
                </button>
                <button class="download-btn" onclick="downloadImageFromCard(this.closest('.image-wrapper').querySelector('img'))" title="下载">
                    <span>⬇</span> 下载
                </button>
            </div>
        </div>
        <div class="image-card-info">
            <span class="image-filename" title="${safeFilename}">${safeFilename}</span>
        </div>
    `;
    
    imageGrid.appendChild(imageCard);
    
    // 触发动画
    setTimeout(() => {
        imageCard.style.opacity = '1';
        imageCard.style.transform = 'scale(1)';
    }, 10);
}

// 下载图像
async function downloadImage(url, filename, fileContent = null, mimeType = 'image/png') {
    try {
        // 如果提供了文件内容（base64），直接使用
        if (fileContent && typeof fileContent === 'string') {
            try {
                // 将base64转换为blob
                const byteCharacters = atob(fileContent);
                const byteNumbers = new Array(byteCharacters.length);
                for (let i = 0; i < byteCharacters.length; i++) {
                    byteNumbers[i] = byteCharacters.charCodeAt(i);
                }
                const byteArray = new Uint8Array(byteNumbers);
                const blob = new Blob([byteArray], { type: mimeType });
                
                const blobUrl = window.URL.createObjectURL(blob);
                const link = document.createElement('a');
                link.href = blobUrl;
                link.download = filename;
                document.body.appendChild(link);
                link.click();
                document.body.removeChild(link);
                
                setTimeout(() => {
                    window.URL.revokeObjectURL(blobUrl);
                }, 100);
                
                showNotification('开始下载: ' + filename, 'success');
                return;
            } catch (err) {
                console.warn('使用文件内容下载失败，尝试使用URL:', err);
            }
        }
        
        // 如果URL是data URL，直接下载
        if (url.startsWith('data:')) {
            const link = document.createElement('a');
            link.href = url;
            link.download = filename;
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            showNotification('开始下载: ' + filename, 'success');
            return;
        }
        
        // 如果URL是相对路径，尝试通过API下载
        if (url.startsWith('/api/') || (!url.startsWith('http') && !url.startsWith('data:'))) {
            // 使用fetch下载文件
            const response = await fetch(url, {
                headers: {
                    'Authorization': `Bearer ${getToken()}`
                }
            });
            
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            
            const blob = await response.blob();
            const blobUrl = window.URL.createObjectURL(blob);
            
            const link = document.createElement('a');
            link.href = blobUrl;
            link.download = filename;
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            
            // 清理blob URL
            setTimeout(() => {
                window.URL.revokeObjectURL(blobUrl);
            }, 100);
            
            showNotification('开始下载: ' + filename, 'success');
        } else {
            // 外部URL，直接下载
            const link = document.createElement('a');
            link.href = url;
            link.download = filename;
            link.target = '_blank';
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            showNotification('开始下载: ' + filename, 'success');
        }
    } catch (error) {
        console.error('下载失败:', error);
        showNotification('下载失败: ' + error.message + '，请右键保存图像', 'error');
    }
}

// 查询任务图像文件列表
async function queryTaskImageFiles(taskId, directory = '', filename = '') {
    try {
        const params = new URLSearchParams();
        if (directory) params.append('directory', directory);
        if (filename) params.append('filename', filename);
        
        const url = `${API_BASE}/tasks/${taskId}/images${params.toString() ? '?' + params.toString() : ''}`;
        const response = await fetch(url, {
            headers: {
                'Authorization': `Bearer ${getToken()}`
            }
        });
        
        const result = await response.json();
        if (result.code === 0) {
            return result.data;
        } else {
            throw new Error(result.message || '查询失败');
        }
    } catch (error) {
        console.error('查询图像文件失败:', error);
        showNotification('查询图像文件失败: ' + error.message, 'error');
        return null;
    }
}

// 加载任务图像文件列表并显示
async function loadTaskImages(taskId) {
    try {
        const result = await queryTaskImageFiles(taskId);
        console.log('查询图像文件结果:', result);
        
        // 处理返回的数据结构：result.data.images 或 result.images
        const imagesArray = result?.images || result?.files || [];
        
        if (imagesArray && imagesArray.length > 0) {
            // 转换文件列表格式，适配 displayImages 函数
            // 优先使用 data_url（如果中台返回了文件内容），否则使用 file_url
            const images = imagesArray
                .filter(file => {
                    // 过滤掉目录，只显示图像文件
                    // 如果文件有 file_type 字段，检查是否为图像；否则检查扩展名
                    if (file.is_dir) return false;
                    if (file.file_type === 'image' || file.file_type?.startsWith('image/')) return true;
                    const filename = file.filename || file.name || '';
                    const ext = filename.toLowerCase().split('.').pop();
                    return ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp'].includes(ext);
                })
                .map(file => {
                    const filename = file.filename || file.name || 'image.png';
                    // 优先使用 data_url（包含base64内容），否则使用 file_url 或 preview_url
                    const imageUrl = file.data_url || file.file_url || file.preview_url || file.url || '';
                    
                    return {
                        filename: filename,
                        // 直接使用 data_url，如果没有则使用其他URL
                        url: imageUrl,
                        data_url: file.data_url || null, // 保存 data_url 用于显示
                        file: {
                            filename: filename,
                            url: imageUrl,
                            data_url: file.data_url || null,
                            file_size: file.file_size || 0,
                            // 保存文件内容相关信息，用于下载
                            file_content: file.file_content || null,
                            content_format: file.content_format || null,
                            mime_type: file.file_type || file.mime_type || 'image/png',
                            file_type: file.file_type || 'image/png'
                        }
                    };
                });
            
            console.log('处理后的图像列表:', images);
            
            if (images.length > 0) {
                displayImages(images);
                // 滚动到图像区域
                const imageSection = document.querySelector('.image-section');
                if (imageSection) {
                    setTimeout(() => {
                        imageSection.scrollIntoView({ behavior: 'smooth', block: 'start' });
                    }, 300);
                }
            } else {
                showNotification('任务完成，但未找到图像文件', 'warning');
            }
        } else {
            console.warn('未找到图像文件，返回结果:', result);
            showNotification('任务完成，但未找到图像文件', 'warning');
        }
    } catch (error) {
        console.error('加载任务图像失败:', error);
        showNotification('加载图像失败: ' + error.message, 'error');
    }
}

// 从卡片元素预览图像（避免在onclick中传递长字符串）
function previewImageFromCard(imgElement) {
    const url = imgElement.getAttribute('data-image-url') || imgElement.src;
    const filename = imgElement.getAttribute('data-image-filename') || imgElement.alt || 'image.png';
    previewImage(url, filename);
}

// 从卡片元素下载图像（避免在onclick中传递长字符串）
function downloadImageFromCard(imgElement) {
    const url = imgElement.getAttribute('data-image-url') || imgElement.src;
    const filename = imgElement.getAttribute('data-image-filename') || imgElement.alt || 'image.png';
    const fileContentStr = imgElement.getAttribute('data-image-content');
    const mimeType = imgElement.getAttribute('data-image-mimetype') || 'image/png';
    
    let fileContent = null;
    if (fileContentStr && fileContentStr !== 'null') {
        try {
            fileContent = JSON.parse(fileContentStr);
        } catch (e) {
            console.warn('解析文件内容失败:', e);
        }
    }
    
    downloadImage(url, filename, fileContent, mimeType);
}

// 预览图像（打开大图预览）
function previewImage(url, filename) {
    // 创建预览模态框
    const modal = document.createElement('div');
    modal.className = 'image-preview-modal';
    modal.style.cssText = `
        position: fixed;
        top: 0;
        left: 0;
        width: 100%;
        height: 100%;
        background: rgba(0,0,0,0.9);
        z-index: 10000;
        display: flex;
        align-items: center;
        justify-content: center;
        cursor: pointer;
    `;
    
    const img = document.createElement('img');
    img.src = url;
    img.style.cssText = `
        max-width: 90%;
        max-height: 90%;
        object-fit: contain;
    `;
    img.alt = filename;
    
    const closeBtn = document.createElement('div');
    closeBtn.innerHTML = '×';
    closeBtn.style.cssText = `
        position: absolute;
        top: 20px;
        right: 30px;
        color: white;
        font-size: 40px;
        cursor: pointer;
        z-index: 10001;
        font-weight: bold;
        line-height: 1;
    `;
    
    const downloadBtn = document.createElement('button');
    downloadBtn.innerHTML = '⬇ 下载';
    downloadBtn.style.cssText = `
        position: absolute;
        bottom: 30px;
        left: 50%;
        transform: translateX(-50%);
        padding: 12px 24px;
        background: #2196f3;
        color: white;
        border: none;
        border-radius: 6px;
        cursor: pointer;
        font-size: 16px;
        z-index: 10001;
        transition: all 0.2s;
    `;
    
    downloadBtn.addEventListener('mouseenter', () => {
        downloadBtn.style.background = '#1976d2';
        downloadBtn.style.transform = 'translateX(-50%) scale(1.05)';
    });
    
    downloadBtn.addEventListener('mouseleave', () => {
        downloadBtn.style.background = '#2196f3';
        downloadBtn.style.transform = 'translateX(-50%) scale(1)';
    });
    
    const closeModal = () => {
        if (document.body.contains(modal)) {
            document.body.removeChild(modal);
        }
    };
    
    modal.addEventListener('click', (e) => {
        if (e.target === modal || e.target === closeBtn) {
            closeModal();
        }
    });
    
    closeBtn.addEventListener('click', closeModal);
    downloadBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        downloadImage(url, filename);
    });
    
    // 按ESC键关闭
    const escHandler = (e) => {
        if (e.key === 'Escape') {
            closeModal();
            document.removeEventListener('keydown', escHandler);
        }
    };
    document.addEventListener('keydown', escHandler);
    
    modal.appendChild(img);
    modal.appendChild(closeBtn);
    modal.appendChild(downloadBtn);
    document.body.appendChild(modal);
}

// 显示通知消息
function showNotification(message, type = 'info') {
    // 移除已存在的通知
    const existingNotification = document.querySelector('.notification');
    if (existingNotification) {
        existingNotification.remove();
    }
    
    // 创建通知元素
    const notification = document.createElement('div');
    notification.className = `notification notification-${type}`;
    notification.textContent = message;
    
    // 添加到页面
    document.body.appendChild(notification);
    
    // 显示动画
    setTimeout(() => {
        notification.classList.add('show');
    }, 10);
    
    // 自动隐藏
    setTimeout(() => {
        notification.classList.remove('show');
        setTimeout(() => {
            notification.remove();
        }, 300);
    }, 3000);
}

// 加载历史记录
async function loadHistory() {
    try {
        const response = await fetch(`${API_BASE}/tasks?page=1&page_size=20`, {
            headers: {
                'Authorization': `Bearer ${getToken()}`
            }
        });
        const result = await response.json();
        
        if (result.code === 0) {
            renderHistoryList(result.data.tasks || []);
        }
    } catch (error) {
        console.error('加载历史记录失败:', error);
    }
}

// 渲染历史记录列表
function renderHistoryList(tasks) {
    const historyList = document.getElementById('historyList');
    if (!historyList) return;

    if (tasks.length === 0) {
        historyList.innerHTML = '<div class="loading">暂无历史记录</div>';
        return;
    }

    historyList.innerHTML = tasks.map(task => {
        const date = new Date(task.created_at).toLocaleString('zh-CN');
        return `
            <div class="history-item" onclick="showHistoryDetail('${task.id}')">
                <div class="history-item-time">${date}</div>
                <div class="history-item-status ${task.status}">${getStatusText(task.status)}</div>
            </div>
        `;
    }).join('');
}

// 获取状态文本
function getStatusText(status) {
    const statusMap = {
        'pending': '等待中',
        'running': '运行中',
        'completed': '已完成',
        'failed': '失败'
    };
    return statusMap[status] || status;
}

// 显示历史记录详情
async function showHistoryDetail(taskId) {
    try {
        const response = await fetch(`${API_BASE}/tasks/${taskId}`, {
            headers: {
                'Authorization': `Bearer ${getToken()}`
            }
        });
        const result = await response.json();
        
        if (result.code === 0) {
            const task = result.data;
            const modal = document.getElementById('historyModal');
            const detail = document.getElementById('historyDetail');
            
            if (modal && detail) {
                // 先显示基本信息
                detail.innerHTML = `
                    <p><strong>任务ID:</strong> ${task.id}</p>
                    <p><strong>状态:</strong> ${getStatusText(task.status)}</p>
                    <p><strong>进度:</strong> ${task.progress || 0}%</p>
                    <p><strong>创建时间:</strong> ${new Date(task.created_at).toLocaleString('zh-CN')}</p>
                    ${task.completed_at ? `<p><strong>完成时间:</strong> ${new Date(task.completed_at).toLocaleString('zh-CN')}</p>` : ''}
                    ${task.error ? `<p><strong>错误:</strong> ${task.error}</p>` : ''}
                    <div class="history-images">
                        <div class="loading">加载图像中...</div>
                    </div>
                `;
                
                modal.style.display = 'block';
                
                // 使用新接口加载图像文件列表
                if (task.status === 'completed') {
                    try {
                        const imageResult = await queryTaskImageFiles(taskId);
                        // 处理返回的数据结构：imageResult.images 或 imageResult.files
                        const imagesArray = imageResult?.images || imageResult?.files || [];
                        if (imagesArray && imagesArray.length > 0) {
                            const images = imagesArray
                                .filter(file => {
                                    if (file.is_dir) return false;
                                    if (file.file_type === 'image' || file.file_type?.startsWith('image/')) return true;
                                    const filename = file.filename || file.name || '';
                                    const ext = filename.toLowerCase().split('.').pop();
                                    return ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp'].includes(ext);
                                })
                                .map(file => ({
                                    filename: file.filename || file.name || 'image.png',
                                    url: file.data_url || file.file_url || file.preview_url || file.url || '',
                                    data_url: file.data_url || null,
                                    file_content: file.file_content || null,
                                    mime_type: file.file_type || file.mime_type || 'image/png'
                                }));
                            
                            const imagesContainer = detail.querySelector('.history-images');
                            if (imagesContainer) {
                                if (images.length > 0) {
                                    imagesContainer.innerHTML = images.map((file, index) => {
                                        const url = file.url || file.data_url || '';
                                        const filename = file.filename || 'image.png';
                                        
                                        // 对于data URL，直接使用；对于普通URL，需要转义
                                        let safeUrl;
                                        if (url.startsWith('data:')) {
                                            safeUrl = url.replace(/'/g, "\\'");
                                        } else {
                                            safeUrl = url.replace(/'/g, "\\'").replace(/"/g, '&quot;');
                                        }
                                        
                                        const safeFilename = filename.replace(/'/g, "\\'").replace(/"/g, '&quot;');
                                        const fileContentJson = file.file_content ? JSON.stringify(file.file_content).replace(/'/g, "\\'").replace(/"/g, '&quot;') : 'null';
                                        const mimeTypeSafe = (file.mime_type || 'image/png').replace(/'/g, "\\'").replace(/"/g, '&quot;');
                                        
                                        // 使用 data-* 属性存储数据
                                        return `
                                            <div class="image-item">
                                                <img src="${safeUrl}" alt="${safeFilename}" 
                                                     data-image-url="${safeUrl}"
                                                     data-image-filename="${safeFilename}"
                                                     data-image-content="${fileContentJson}"
                                                     data-image-mimetype="${mimeTypeSafe}"
                                                     onclick="previewImageFromCard(this)" 
                                                     style="cursor: pointer;">
                                                <button onclick="downloadImageFromCard(this.previousElementSibling)">下载</button>
                                            </div>
                                        `;
                                    }).join('');
                                } else {
                                    imagesContainer.innerHTML = '<div class="empty-state">暂无图像</div>';
                                }
                            }
                        } else {
                            // 如果没有通过新接口获取到图像，尝试使用旧的数据
                            const imagesContainer = detail.querySelector('.history-images');
                            if (imagesContainer) {
                                if (task.files && task.files.length > 0) {
                                    imagesContainer.innerHTML = task.files.map(file => {
                                        const safeUrl = (file.url || '').replace(/'/g, "\\'").replace(/"/g, '&quot;');
                                        const safeFilename = (file.filename || '').replace(/'/g, "\\'").replace(/"/g, '&quot;');
                                        return `
                                            <div class="image-item">
                                                <img src="${safeUrl}" alt="${safeFilename}" 
                                                     onclick="previewImage('${safeUrl}', '${safeFilename}')" 
                                                     style="cursor: pointer;">
                                                <button onclick="downloadImage('${safeUrl}', '${safeFilename}')">下载</button>
                                            </div>
                                        `;
                                    }).join('');
                                } else {
                                    imagesContainer.innerHTML = '<div class="empty-state">暂无图像</div>';
                                }
                            }
                        }
                    } catch (imageError) {
                        console.error('加载图像文件失败:', imageError);
                        const imagesContainer = detail.querySelector('.history-images');
                        if (imagesContainer) {
                            // 回退到使用旧数据
                            if (task.files && task.files.length > 0) {
                                imagesContainer.innerHTML = task.files.map(file => {
                                    const safeUrl = (file.url || '').replace(/'/g, "\\'").replace(/"/g, '&quot;');
                                    const safeFilename = (file.filename || '').replace(/'/g, "\\'").replace(/"/g, '&quot;');
                                    return `
                                        <div class="image-item">
                                            <img src="${safeUrl}" alt="${safeFilename}" 
                                                 onclick="previewImage('${safeUrl}', '${safeFilename}')" 
                                                 style="cursor: pointer;">
                                            <button onclick="downloadImage('${safeUrl}', '${safeFilename}')">下载</button>
                                        </div>
                                    `;
                                }).join('');
                            } else {
                                imagesContainer.innerHTML = '<div class="empty-state">加载图像失败</div>';
                            }
                        }
                    }
                } else {
                    const imagesContainer = detail.querySelector('.history-images');
                    if (imagesContainer) {
                        imagesContainer.innerHTML = '<div class="empty-state">任务未完成，暂无图像</div>';
                    }
                }
            }
        }
    } catch (error) {
        console.error('加载任务详情失败:', error);
        alert('加载任务详情失败: ' + error.message);
    }
}

// 关闭模态框
document.addEventListener('DOMContentLoaded', function() {
    const modal = document.getElementById('historyModal');
    const closeBtn = modal?.querySelector('.close');
    
    if (closeBtn) {
        closeBtn.onclick = function() {
            modal.style.display = 'none';
        };
    }
    
    if (modal) {
        window.onclick = function(event) {
            if (event.target === modal) {
                modal.style.display = 'none';
            }
        };
    }

    // 绑定生成按钮
    const generateBtn = document.getElementById('generateBtn');
    if (generateBtn) {
        generateBtn.onclick = submitTask;
    }
});

// 页面加载时初始化
async function init() {
    try {
        await ensureUser();
        await loadWorkflows(); // 加载工作流列表
        loadHistory();
        // WebSocket连接已移除，改用定时查询机制
    } catch (error) {
        console.error('初始化失败:', error);
        alert('初始化失败: ' + error.message);
    }
}

window.addEventListener('DOMContentLoaded', init);

