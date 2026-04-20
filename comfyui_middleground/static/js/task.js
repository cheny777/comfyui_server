let currentPage = 1;
let pageSize = 20;

async function loadTasks() {
    const status = document.getElementById('statusFilter').value;
    const userID = document.getElementById('userFilter').value;
    const search = document.getElementById('searchInput').value;

    const params = new URLSearchParams({
        page: currentPage,
        page_size: pageSize,
    });
    if (status) params.append('status', status);
    if (userID) params.append('user_id', userID);
    if (search) params.append('search', search);

    try {
        const response = await fetch(`/api/tasks?${params}`);
        const result = await response.json();

        if (result.code === 0) {
            renderTasks(result.data.tasks);
            updatePagination(result.data.total, result.data.page);
        }
    } catch (error) {
        console.error('加载任务列表失败:', error);
    }
}

function renderTasks(tasks) {
    const tbody = document.getElementById('taskTableBody');
    tbody.innerHTML = '';

    tasks.forEach(task => {
        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td>${task.id}</td>
            <td>${task.request_id}</td>
            <td>${task.user_id}</td>
            <td>${task.device_id || '-'}</td>
            <td>${task.prompt_text ? task.prompt_text.substring(0, 50) + '...' : '-'}</td>
            <td><span class="status-${task.status}">${task.status}</span></td>
            <td>${task.progress}%</td>
            <td>${new Date(task.created_at).toLocaleString()}</td>
            <td><button onclick="showTaskDetail('${task.request_id}')">查看</button></td>
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
        loadTasks();
    }
}

function nextPage() {
    currentPage++;
    loadTasks();
}

function refreshTasks() {
    loadTasks();
}

async function showTaskDetail(requestID) {
    try {
        const response = await fetch(`/api/tasks/${requestID}`);
        const result = await response.json();

        if (result.code === 0) {
            const task = result.data;
            const modal = document.getElementById('taskDetailModal');
            const content = document.getElementById('taskDetailContent');

            let filesHTML = '';
            if (task.files_info) {
                try {
                    const files = JSON.parse(task.files_info);
                    filesHTML = files.map(file => `
                        <div class="file-item">
                            <img src="${file.url}" alt="${file.filename}" style="max-width: 200px;">
                            <p>${file.filename}</p>
                        </div>
                    `).join('');
                } catch (e) {
                    filesHTML = '<p>无法解析文件信息</p>';
                }
            }

            content.innerHTML = `
                <p><strong>请求ID:</strong> ${task.request_id}</p>
                <p><strong>用户ID:</strong> ${task.user_id}</p>
                <p><strong>设备ID:</strong> ${task.device_id || '-'}</p>
                <p><strong>Prompt ID:</strong> ${task.prompt_id || '-'}</p>
                <p><strong>状态:</strong> ${task.status}</p>
                <p><strong>进度:</strong> ${task.progress}%</p>
                <p><strong>正向提示词:</strong> ${task.prompt_text || '-'}</p>
                <p><strong>负向提示词:</strong> ${task.negative_prompt || '-'}</p>
                <p><strong>创建时间:</strong> ${new Date(task.created_at).toLocaleString()}</p>
                <h3>生成的图像:</h3>
                <div class="files-list">${filesHTML || '<p>暂无图像</p>'}</div>
            `;

            modal.style.display = 'block';
        }
    } catch (error) {
        console.error('获取任务详情失败:', error);
    }
}

function closeTaskDetail() {
    document.getElementById('taskDetailModal').style.display = 'none';
}

// 页面加载时初始化
window.addEventListener('DOMContentLoaded', () => {
    loadTasks();
    // 每5秒自动刷新
    setInterval(loadTasks, 5000);
});

