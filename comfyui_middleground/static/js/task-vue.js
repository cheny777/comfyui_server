const { createApp } = Vue;
const { ElMessage } = ElementPlus;
const { Picture } = ElementPlusIconsVue;

const app = createApp({
    components: {
        Picture
    },
    data() {
        return {
            activeMenu: '/',
            // 筛选条件
            filters: {
                status: '',
                user_id: '',
                search: ''
            },
            // 任务列表
            tasks: [],
            // 分页信息
            pagination: {
                page: 1,
                pageSize: 20,
                total: 0
            },
            // 加载状态
            loading: false,
            // 详情对话框
            detailVisible: false,
            currentTask: null
        };
    },
    mounted() {
        // 根据当前路径设置activeMenu
        this.activeMenu = window.location.pathname;
        this.loadTasks();
    },
    methods: {
        // 加载任务列表
        async loadTasks() {
            this.loading = true;
            try {
                const params = new URLSearchParams({
                    page: this.pagination.page,
                    page_size: this.pagination.pageSize
                });

                if (this.filters.status) {
                    params.append('status', this.filters.status);
                }
                if (this.filters.user_id) {
                    params.append('user_id', this.filters.user_id);
                }
                if (this.filters.search) {
                    params.append('search', this.filters.search);
                }

                const response = await fetch(`/api/tasks?${params}`);
                const result = await response.json();

                if (result.code === 0) {
                    this.tasks = result.data.tasks || [];
                    this.pagination.total = result.data.total || 0;
                } else {
                    ElMessage.error('加载任务列表失败: ' + result.message);
                }
            } catch (error) {
                console.error('加载任务列表失败:', error);
                ElMessage.error('加载任务列表失败: ' + error.message);
            } finally {
                this.loading = false;
            }
        },
        // 搜索
        handleSearch() {
            this.pagination.page = 1;
            this.loadTasks();
        },
        // 重置
        handleReset() {
            this.filters = {
                status: '',
                user_id: '',
                search: ''
            };
            this.pagination.page = 1;
            this.loadTasks();
        },
        // 分页变化
        handlePageChange(page) {
            this.pagination.page = page;
            this.loadTasks();
        },
        // 每页数量变化
        handleSizeChange(size) {
            this.pagination.pageSize = size;
            this.pagination.page = 1;
            this.loadTasks();
        },
        // 查看详情
        async handleViewDetail(task) {
            try {
                const response = await fetch(`/api/tasks/${task.request_id}`);
                const result = await response.json();
                if (result.code === 0) {
                    this.currentTask = result.data;
                    this.detailVisible = true;
                } else {
                    ElMessage.error('加载任务详情失败: ' + result.message);
                }
            } catch (error) {
                console.error('加载任务详情失败:', error);
                ElMessage.error('加载任务详情失败: ' + error.message);
            }
        },
        // 查看图像
        handleViewImages(task) {
            window.location.href = `/images?task_id=${task.request_id}`;
        },
        // 复制任务ID
        async copyTaskId(taskId) {
            try {
                await navigator.clipboard.writeText(taskId);
                ElMessage.success('已复制到剪贴板');
            } catch (error) {
                console.error('复制失败:', error);
                ElMessage.error('复制失败，请手动复制');
            }
        },
        // 菜单选择
        handleMenuSelect(index) {
            if (index !== this.activeMenu) {
                window.location.href = index;
            }
        },
        // 工具函数
        truncateText(text, maxLength) {
            if (!text) return '';
            return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
        },
        formatDate(dateStr) {
            if (!dateStr) return '';
            return new Date(dateStr).toLocaleString('zh-CN');
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
        // 获取任务图像
        getTaskImage(task) {
            if (!task.files_info) return null;
            try {
                const files = JSON.parse(task.files_info);
                if (files && files.length > 0 && files[0].url) {
                    return files[0].url;
                }
            } catch (e) {
                console.error('解析files_info失败:', e);
            }
            return null;
        },
        // 获取任务图像数量
        getTaskImageCount(task) {
            if (!task.files_info) return 0;
            try {
                const files = JSON.parse(task.files_info);
                return files ? files.length : 0;
            } catch (e) {
                return 0;
            }
        },
        // 图像加载错误处理
        handleImageError(event) {
            event.target.style.display = 'none';
            const placeholder = event.target.parentElement.querySelector('.task-placeholder');
            if (placeholder) {
                placeholder.style.display = 'flex';
            }
        }
    }
});

app.use(ElementPlus);
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
    app.component(key, component);
}
app.mount('#app');

