const { createApp } = Vue;
const { ElMessage, ElMessageBox } = ElementPlus;

const app = createApp({
    data() {
        return {
            activeMenu: '/images',
            // 筛选条件
            filters: {
                user_id: '',
                task_id: '',
                date_from: '',
                date_to: ''
            },
            // 视图模式
            viewMode: localStorage.getItem('imageView') || 'grid',
            // 图像列表
            images: [],
            // 分页信息
            pagination: {
                page: 1,
                pageSize: 24,
                total: 0
            },
            // 选中的图像
            selectedImages: [],
            selectAll: false,
            // 预览
            previewVisible: false,
            previewImage: null,
            // 加载状态
            loading: false
        };
    },
    mounted() {
        // 根据当前路径设置activeMenu
        this.activeMenu = window.location.pathname;
        this.loadImages();
    },
    methods: {
        // 加载图像列表
        async loadImages() {
            this.loading = true;
            try {
                const params = new URLSearchParams({
                    page: this.pagination.page,
                    page_size: this.pagination.pageSize
                });

                if (this.filters.user_id) {
                    params.append('user_id', this.filters.user_id);
                }
                if (this.filters.task_id) {
                    params.append('task_id', this.filters.task_id);
                }
                if (this.filters.date_from) {
                    params.append('date_from', this.filters.date_from);
                }
                if (this.filters.date_to) {
                    params.append('date_to', this.filters.date_to);
                }

                const response = await fetch(`/api/images?${params}`);
                const result = await response.json();

                if (result.code === 0) {
                    this.images = result.data.images || [];
                    this.pagination.total = result.data.total || 0;
                } else {
                    ElMessage.error('加载图像列表失败: ' + result.message);
                }
            } catch (error) {
                console.error('加载图像列表失败:', error);
                ElMessage.error('加载图像列表失败: ' + error.message);
            } finally {
                this.loading = false;
            }
        },
        // 搜索
        handleSearch() {
            this.pagination.page = 1;
            this.loadImages();
        },
        // 重置
        handleReset() {
            this.filters = {
                user_id: '',
                task_id: '',
                date_from: '',
                date_to: ''
            };
            this.pagination.page = 1;
            this.loadImages();
        },
        // 分页变化
        handlePageChange(page) {
            this.pagination.page = page;
            this.loadImages();
            // 滚动到顶部
            window.scrollTo({ top: 0, behavior: 'smooth' });
        },
        // 每页数量变化
        handleSizeChange(size) {
            this.pagination.pageSize = size;
            this.pagination.page = 1;
            this.loadImages();
        },
        // 获取图像URL
        getImageUrl(image) {
            return image.url || `/api/image-file/${image.user_id}/${image.request_id}/${image.filename}`;
        },
        // 图像加载错误
        handleImageError(event) {
            event.target.src = 'data:image/svg+xml,%3Csvg xmlns=\'http://www.w3.org/2000/svg\' width=\'200\' height=\'200\'%3E%3Crect fill=\'%23ddd\' width=\'200\' height=\'200\'/%3E%3Ctext fill=\'%23999\' font-family=\'sans-serif\' font-size=\'14\' x=\'50%25\' y=\'50%25\' text-anchor=\'middle\' dy=\'.3em\'%3E加载失败%3C/text%3E%3C/svg%3E';
        },
        // 选择图像
        handleSelectImage(imageId, selected) {
            if (selected) {
                if (!this.selectedImages.includes(imageId)) {
                    this.selectedImages.push(imageId);
                }
            } else {
                const index = this.selectedImages.indexOf(imageId);
                if (index > -1) {
                    this.selectedImages.splice(index, 1);
                }
            }
            this.updateSelectAll();
        },
        // 是否已选择
        isSelected(imageId) {
            return this.selectedImages.includes(imageId);
        },
        // 全选/取消全选
        handleSelectAll(checked) {
            if (checked) {
                this.selectedImages = this.images.map(img => img.id);
            } else {
                this.selectedImages = [];
            }
        },
        // 更新全选状态
        updateSelectAll() {
            this.selectAll = this.images.length > 0 && this.selectedImages.length === this.images.length;
        },
        // 预览图像
        handlePreview(image, index) {
            this.previewImage = image;
            this.previewVisible = true;
        },
        // 下载图像
        handleDownload(image) {
            const url = this.getImageUrl(image);
            const link = document.createElement('a');
            link.href = url;
            link.download = image.filename;
            link.target = '_blank';
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            ElMessage.success('开始下载: ' + image.filename);
        },
        // 批量下载
        handleBatchDownload() {
            if (this.selectedImages.length === 0) {
                ElMessage.warning('请先选择要下载的图像');
                return;
            }

            const selectedImageObjects = this.images.filter(img => this.selectedImages.includes(img.id));
            selectedImageObjects.forEach((image, index) => {
                setTimeout(() => {
                    this.handleDownload(image);
                }, index * 100);
            });
            ElMessage.success(`开始批量下载 ${selectedImageObjects.length} 张图像`);
        },
        // 批量删除
        async handleBatchDelete() {
            if (this.selectedImages.length === 0) {
                ElMessage.warning('请先选择要删除的图像');
                return;
            }

            try {
                await ElMessageBox.confirm(
                    `确定要删除选中的 ${this.selectedImages.length} 张图像吗？此操作不可恢复。`,
                    '确认删除',
                    {
                        confirmButtonText: '确定',
                        cancelButtonText: '取消',
                        type: 'warning'
                    }
                );
                // TODO: 实现删除API
                ElMessage.info('删除功能待实现');
            } catch {
                // 用户取消
            }
        },
        // 查看任务
        handleViewTask(image) {
            window.location.href = `/?search=${image.request_id}`;
        },
        // 复制到剪贴板
        async copyToClipboard(text) {
            try {
                await navigator.clipboard.writeText(text);
                ElMessage.success('已复制到剪贴板');
            } catch (error) {
                console.error('复制失败:', error);
                ElMessage.error('复制失败，请手动复制');
            }
        },
        // 工具函数
        formatFileSize(bytes) {
            if (!bytes || bytes === 0) return '0 B';
            const k = 1024;
            const sizes = ['B', 'KB', 'MB', 'GB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
        },
        formatDate(dateStr) {
            if (!dateStr) return '';
            return new Date(dateStr).toLocaleString('zh-CN');
        },
        truncateText(text, maxLength) {
            if (!text) return '';
            return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
        },
        getStatusText(status) {
            const statusMap = {
                'pending': '待处理',
                'running': '运行中',
                'completed': '已完成',
                'failed': '失败'
            };
            return statusMap[status] || status;
        },
        getStatusType(status) {
            const typeMap = {
                'pending': 'info',
                'running': 'warning',
                'completed': 'success',
                'failed': 'danger'
            };
            return typeMap[status] || '';
        },
        // 菜单选择
        handleMenuSelect(index) {
            if (index !== this.activeMenu) {
                window.location.href = index;
            }
        }
    },
    watch: {
        viewMode(newVal) {
            localStorage.setItem('imageView', newVal);
        },
        images() {
            this.updateSelectAll();
        }
    }
});

app.use(ElementPlus).mount('#app');

