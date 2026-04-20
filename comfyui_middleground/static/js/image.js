// 全局变量
let currentPage = 1;
let pageSize = 24;
let totalPages = 1;
let selectedImages = new Set();

// 页面加载时初始化
window.addEventListener('DOMContentLoaded', () => {
    loadImages();
    setupEventListeners();
});

// 设置事件监听器
function setupEventListeners() {
    // 筛选按钮
    document.getElementById('searchBtn').addEventListener('click', () => {
        currentPage = 1;
        loadImages();
    });

    // 重置按钮
    document.getElementById('resetBtn').addEventListener('click', () => {
        document.getElementById('imageUserFilter').value = '';
        document.getElementById('imageTaskFilter').value = '';
        document.getElementById('dateFrom').value = '';
        document.getElementById('dateTo').value = '';
        currentPage = 1;
        loadImages();
    });

    // 全选/取消全选
    document.getElementById('selectAll').addEventListener('change', (e) => {
        const checked = e.target.checked;
        document.querySelectorAll('.image-checkbox').forEach(cb => {
            cb.checked = checked;
            if (checked) {
                selectedImages.add(cb.value);
            } else {
                selectedImages.delete(cb.value);
            }
        });
        updateSelectionInfo();
    });

    // 批量操作按钮
    document.getElementById('batchDownload').addEventListener('click', batchDownload);
    document.getElementById('batchDelete').addEventListener('click', batchDelete);

    // 视图切换
    document.querySelectorAll('.view-toggle').forEach(btn => {
        btn.addEventListener('click', (e) => {
            document.querySelectorAll('.view-toggle').forEach(b => b.classList.remove('active'));
            e.target.classList.add('active');
            const view = e.target.dataset.view;
            document.getElementById('imageGrid').className = `image-grid ${view}`;
            localStorage.setItem('imageView', view);
        });
    });

    // 恢复保存的视图
    const savedView = localStorage.getItem('imageView') || 'grid';
    document.querySelector(`[data-view="${savedView}"]`).classList.add('active');
    document.getElementById('imageGrid').className = `image-grid ${savedView}`;

    // 分页按钮
    document.getElementById('prevPage').addEventListener('click', () => {
        if (currentPage > 1) {
            currentPage--;
            loadImages();
        }
    });

    document.getElementById('nextPage').addEventListener('click', () => {
        if (currentPage < totalPages) {
            currentPage++;
            loadImages();
        }
    });

    // 模态框关闭
    document.getElementById('imagePreviewModal').addEventListener('click', (e) => {
        if (e.target.id === 'imagePreviewModal') {
            closeImagePreview();
        }
    });

    // ESC键关闭模态框
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') {
            closeImagePreview();
        }
    });
}

// 加载图像列表
async function loadImages() {
    try {
        showLoading(true);
        
        const userID = document.getElementById('imageUserFilter').value;
        const taskID = document.getElementById('imageTaskFilter').value;
        const dateFrom = document.getElementById('dateFrom').value;
        const dateTo = document.getElementById('dateTo').value;

        const params = new URLSearchParams({
            page: currentPage,
            page_size: pageSize,
        });

        if (userID) params.append('user_id', userID);
        if (taskID) params.append('task_id', taskID);
        if (dateFrom) params.append('date_from', dateFrom);
        if (dateTo) params.append('date_to', dateTo);

        const response = await fetch(`/api/images?${params}`);
        const result = await response.json();

        if (result.code === 0) {
            renderImages(result.data.images || []);
            updatePagination(result.data);
        } else {
            showError('加载图像列表失败: ' + result.message);
        }
    } catch (error) {
        console.error('加载图像列表失败:', error);
        showError('加载图像列表失败: ' + error.message);
    } finally {
        showLoading(false);
    }
}

// 渲染图像列表
function renderImages(images) {
    const grid = document.getElementById('imageGrid');
    grid.innerHTML = '';

    if (images.length === 0) {
        grid.innerHTML = '<div class="empty-state">暂无图像</div>';
        return;
    }

    images.forEach((image, index) => {
        const card = createImageCard(image, index);
        grid.appendChild(card);
    });

    updateSelectionInfo();
}

// 创建图像卡片
function createImageCard(image, index) {
    const card = document.createElement('div');
    card.className = 'image-card';
    card.dataset.imageId = image.id;
    
    const imageUrl = image.url || `/api/image-file/${image.user_id}/${image.request_id}/${image.filename}`;
    const createdAt = new Date(image.created_at).toLocaleString('zh-CN');
    const fileSize = formatFileSize(image.file_size || 0);
    const dimensions = image.width && image.height ? `${image.width} × ${image.height}` : '未知';

    card.innerHTML = `
        <div class="image-card-header">
            <input type="checkbox" class="image-checkbox" value="${image.id}" 
                   onchange="toggleImageSelection(${image.id}, this.checked)">
            <div class="image-actions">
                <button class="icon-btn" onclick="previewImage(${index})" title="预览">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                        <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path>
                        <circle cx="12" cy="12" r="3"></circle>
                    </svg>
                </button>
                <button class="icon-btn" onclick="downloadImage('${imageUrl}', '${image.filename}')" title="下载">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
                        <polyline points="7 10 12 15 17 10"></polyline>
                        <line x1="12" y1="15" x2="12" y2="3"></line>
                    </svg>
                </button>
            </div>
        </div>
        <div class="image-card-body" onclick="previewImage(${index})">
            <img src="${imageUrl}" alt="${image.filename}" loading="lazy" 
                 onerror="this.src='data:image/svg+xml,%3Csvg xmlns=\'http://www.w3.org/2000/svg\' width=\'200\' height=\'200\'%3E%3Crect fill=\'%23ddd\' width=\'200\' height=\'200\'/%3E%3Ctext fill=\'%23999\' font-family=\'sans-serif\' font-size=\'14\' x=\'50%25\' y=\'50%25\' text-anchor=\'middle\' dy=\'.3em\'%3E加载失败%3C/text%3E%3C/svg%3E'">
            <div class="image-overlay">
                <div class="image-info-badge">
                    <span>${fileSize}</span>
                    <span>${dimensions}</span>
                </div>
            </div>
        </div>
        <div class="image-card-footer">
            <div class="image-title" title="${image.filename}">${truncateText(image.filename, 20)}</div>
            <div class="image-meta">
                <span class="meta-item">用户: ${image.user_id}</span>
                <span class="meta-item">${createdAt}</span>
            </div>
            ${image.prompt_text ? `<div class="image-prompt" title="${image.prompt_text}">${truncateText(image.prompt_text, 30)}</div>` : ''}
        </div>
    `;

    // 存储图像数据到全局数组
    if (!window.imageList) {
        window.imageList = [];
    }
    window.imageList[index] = image;

    return card;
}

// 切换图像选择
function toggleImageSelection(imageId, checked) {
    if (checked) {
        selectedImages.add(imageId);
    } else {
        selectedImages.delete(imageId);
    }
    updateSelectionInfo();
    updateSelectAllCheckbox();
}

// 更新选择信息
function updateSelectionInfo() {
    const count = selectedImages.size;
    const info = document.getElementById('selectionInfo');
    if (count > 0) {
        info.textContent = `已选择 ${count} 张图像`;
        info.style.display = 'block';
        document.getElementById('batchActions').style.display = 'flex';
    } else {
        info.style.display = 'none';
        document.getElementById('batchActions').style.display = 'none';
    }
}

// 更新全选复选框
function updateSelectAllCheckbox() {
    const checkboxes = document.querySelectorAll('.image-checkbox');
    const checkedCount = Array.from(checkboxes).filter(cb => cb.checked).length;
    const selectAll = document.getElementById('selectAll');
    selectAll.checked = checkedCount === checkboxes.length && checkboxes.length > 0;
    selectAll.indeterminate = checkedCount > 0 && checkedCount < checkboxes.length;
}

// 预览图像
function previewImage(index) {
    const image = window.imageList[index];
    if (!image) return;

    const modal = document.getElementById('imagePreviewModal');
    const imageUrl = image.url || `/api/image-file/${image.user_id}/${image.request_id}/${image.filename}`;
    
    document.getElementById('previewImage').src = imageUrl;
    document.getElementById('previewFilename').textContent = image.filename;
    document.getElementById('previewSize').textContent = formatFileSize(image.file_size || 0);
    document.getElementById('previewDimensions').textContent = 
        image.width && image.height ? `${image.width} × ${image.height}` : '未知';
    document.getElementById('previewTime').textContent = new Date(image.created_at).toLocaleString('zh-CN');
    document.getElementById('previewUserID').textContent = image.user_id;
    document.getElementById('previewTaskID').textContent = image.request_id;
    document.getElementById('previewPrompt').textContent = image.prompt_text || '无';
    document.getElementById('previewStatus').textContent = getStatusText(image.status);
    
    // 设置下载按钮
    document.getElementById('downloadBtn').onclick = () => {
        downloadImage(imageUrl, image.filename);
    };

    // 设置查看任务按钮
    document.getElementById('viewTaskBtn').onclick = () => {
        window.location.href = `/?search=${image.request_id}`;
    };

    modal.style.display = 'flex';
    document.body.style.overflow = 'hidden';
}

// 关闭预览
function closeImagePreview() {
    document.getElementById('imagePreviewModal').style.display = 'none';
    document.body.style.overflow = '';
}

// 下载图像
function downloadImage(url, filename) {
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    link.target = '_blank';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
}

// 批量下载
function batchDownload() {
    if (selectedImages.size === 0) {
        alert('请先选择要下载的图像');
        return;
    }

    const checkboxes = document.querySelectorAll('.image-checkbox:checked');
    checkboxes.forEach(cb => {
        const card = cb.closest('.image-card');
        const image = window.imageList[Array.from(document.querySelectorAll('.image-card')).indexOf(card)];
        if (image) {
            const imageUrl = image.url || `/api/image-file/${image.user_id}/${image.request_id}/${image.filename}`;
            setTimeout(() => downloadImage(imageUrl, image.filename), 100);
        }
    });
}

// 批量删除（提示）
function batchDelete() {
    if (selectedImages.size === 0) {
        alert('请先选择要删除的图像');
        return;
    }

    if (!confirm(`确定要删除选中的 ${selectedImages.size} 张图像吗？此操作不可恢复。`)) {
        return;
    }

    // TODO: 实现删除API
    alert('删除功能待实现');
}

// 更新分页信息
function updatePagination(data) {
    totalPages = Math.ceil(data.total / pageSize);
    document.getElementById('pageInfo').textContent = 
        `第 ${currentPage} 页，共 ${totalPages} 页，共 ${data.image_count || 0} 张图像`;
    
    document.getElementById('prevPage').disabled = currentPage === 1;
    document.getElementById('nextPage').disabled = currentPage >= totalPages;
}

// 工具函数
function formatFileSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}

function truncateText(text, maxLength) {
    if (!text) return '';
    return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
}

function getStatusText(status) {
    const statusMap = {
        'pending': '待处理',
        'running': '运行中',
        'completed': '已完成',
        'failed': '失败'
    };
    return statusMap[status] || status;
}

function showLoading(show) {
    const grid = document.getElementById('imageGrid');
    if (show) {
        grid.innerHTML = '<div class="loading-state">加载中...</div>';
    }
}

function showError(message) {
    const grid = document.getElementById('imageGrid');
    grid.innerHTML = `<div class="error-state">${message}</div>`;
}

// 复制到剪贴板
function copyToClipboard(text) {
    navigator.clipboard.writeText(text).then(() => {
        // 显示提示
        const originalText = event.target.textContent;
        event.target.textContent = '已复制!';
        event.target.style.color = '#28a745';
        setTimeout(() => {
            event.target.textContent = originalText;
            event.target.style.color = '';
        }, 2000);
    }).catch(err => {
        console.error('复制失败:', err);
        alert('复制失败，请手动复制');
    });
}
