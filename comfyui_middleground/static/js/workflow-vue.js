const { createApp } = Vue;
const { ElMessage, ElMessageBox } = ElementPlus;

const app = createApp({
    data() {
        return {
            activeMenu: '/workflows',
            // 筛选条件
            filters: {
                category: '',
                search: ''
            },
            // 工作流列表
            workflows: [],
            // 分页信息
            pagination: {
                page: 1,
                pageSize: 20,
                total: 0
            },
            // 加载状态
            loading: false,
            // 表单对话框
            formVisible: false,
            formMode: 'create', // 'create' or 'edit'
            workflowFormRef: null,
            workflowForm: {
                id: null,
                name: '',
                description: '',
                category: '',
                tags: '',
                is_default: false,
                is_public: true,
                positive_node_id: '',
                positive_field_name: 'text',
                negative_node_id: '',
                negative_field_name: 'text',
                workflow_json: ''
            },
            // 详情对话框
            detailVisible: false,
            currentWorkflow: null
        };
    },
    mounted() {
        // 根据当前路径设置activeMenu
        this.activeMenu = window.location.pathname;
        this.loadWorkflows();
    },
    methods: {
        // 加载工作流列表
        async loadWorkflows() {
            this.loading = true;
            try {
                const params = new URLSearchParams({
                    page: this.pagination.page,
                    page_size: this.pagination.pageSize
                });

                if (this.filters.category) {
                    params.append('category', this.filters.category);
                }
                if (this.filters.search) {
                    params.append('search', this.filters.search);
                }

                const response = await fetch(`/api/workflows?${params}`);
                const result = await response.json();

                if (result.code === 0) {
                    this.workflows = result.data.workflows || [];
                    this.pagination.total = result.data.total || 0;
                } else {
                    ElMessage.error('加载工作流列表失败: ' + result.message);
                }
            } catch (error) {
                console.error('加载工作流列表失败:', error);
                ElMessage.error('加载工作流列表失败: ' + error.message);
            } finally {
                this.loading = false;
            }
        },
        // 搜索
        handleSearch() {
            this.pagination.page = 1;
            this.loadWorkflows();
        },
        // 重置
        handleReset() {
            this.filters = {
                category: '',
                search: ''
            };
            this.pagination.page = 1;
            this.loadWorkflows();
        },
        // 分页变化
        handlePageChange(page) {
            this.pagination.page = page;
            this.loadWorkflows();
        },
        // 每页数量变化
        handleSizeChange(size) {
            this.pagination.pageSize = size;
            this.pagination.page = 1;
            this.loadWorkflows();
        },
        // 创建
        handleCreate() {
            this.formMode = 'create';
            this.workflowForm = {
                id: null,
                name: '',
                description: '',
                category: '',
                tags: '',
                is_default: false,
                is_public: true,
                positive_node_id: '',
                positive_field_name: 'text',
                negative_node_id: '',
                negative_field_name: 'text',
                workflow_json: ''
            };
            this.formVisible = true;
        },
        // 编辑
        async handleEdit(workflow) {
            try {
                const response = await fetch(`/api/workflows/${workflow.id}`);
                const result = await response.json();
                if (result.code === 0) {
                    const wf = result.data;
                    this.formMode = 'edit';
                    this.workflowForm = {
                        id: wf.id,
                        name: wf.name,
                        description: wf.description || '',
                        category: wf.category || '',
                        tags: wf.tags || '',
                        is_default: wf.is_default || false,
                        is_public: wf.is_public !== false,
                        positive_node_id: wf.positive_node_id || '',
                        positive_field_name: wf.positive_field_name || 'text',
                        negative_node_id: wf.negative_node_id || '',
                        negative_field_name: wf.negative_field_name || 'text',
                        workflow_json: this.formatJSON(wf.workflow_json)
                    };
                    this.formVisible = true;
                } else {
                    ElMessage.error('加载工作流详情失败: ' + result.message);
                }
            } catch (error) {
                console.error('加载工作流详情失败:', error);
                ElMessage.error('加载工作流详情失败: ' + error.message);
            }
        },
        // 查看详情
        async handleViewDetail(workflow) {
            try {
                const response = await fetch(`/api/workflows/${workflow.id}`);
                const result = await response.json();
                if (result.code === 0) {
                    this.currentWorkflow = result.data;
                    this.detailVisible = true;
                } else {
                    ElMessage.error('加载工作流详情失败: ' + result.message);
                }
            } catch (error) {
                console.error('加载工作流详情失败:', error);
                ElMessage.error('加载工作流详情失败: ' + error.message);
            }
        },
        // 删除
        async handleDelete(workflow) {
            try {
                await ElMessageBox.confirm(
                    `确定要删除工作流 "${workflow.name}" 吗？此操作不可恢复。`,
                    '确认删除',
                    {
                        confirmButtonText: '确定',
                        cancelButtonText: '取消',
                        type: 'warning'
                    }
                );

                const response = await fetch(`/api/workflows/${workflow.id}`, {
                    method: 'DELETE'
                });
                const result = await response.json();

                if (result.code === 0) {
                    ElMessage.success('删除成功');
                    this.loadWorkflows();
                } else {
                    ElMessage.error('删除失败: ' + result.message);
                }
            } catch (error) {
                if (error !== 'cancel') {
                    console.error('删除工作流失败:', error);
                    ElMessage.error('删除工作流失败: ' + error.message);
                }
            }
        },
        // 执行
        handleExecute(workflow) {
            window.location.href = `/test?workflow_id=${workflow.id}`;
        },
        // 提交表单
        async handleSubmit() {
            if (!this.$refs.workflowFormRef) return;
            
            await this.$refs.workflowFormRef.validate(async (valid) => {
                if (!valid) return;

                try {
                    const url = this.formMode === 'create' 
                        ? '/api/workflows' 
                        : `/api/workflows/${this.workflowForm.id}`;
                    const method = this.formMode === 'create' ? 'POST' : 'PUT';

                    const response = await fetch(url, {
                        method: method,
                        headers: {
                            'Content-Type': 'application/json'
                        },
                        body: JSON.stringify({
                            name: this.workflowForm.name,
                            description: this.workflowForm.description,
                            category: this.workflowForm.category,
                            tags: this.workflowForm.tags,
                            is_default: this.workflowForm.is_default,
                            is_public: this.workflowForm.is_public,
                            positive_node_id: this.workflowForm.positive_node_id,
                            positive_field_name: this.workflowForm.positive_field_name,
                            negative_node_id: this.workflowForm.negative_node_id,
                            negative_field_name: this.workflowForm.negative_field_name,
                            workflow_json: this.workflowForm.workflow_json
                        })
                    });

                    const result = await response.json();

                    if (result.code === 0) {
                        ElMessage.success(this.formMode === 'create' ? '创建成功' : '更新成功');
                        this.formVisible = false;
                        this.loadWorkflows();
                    } else {
                        ElMessage.error('保存失败: ' + result.message);
                    }
                } catch (error) {
                    console.error('保存工作流失败:', error);
                    ElMessage.error('保存工作流失败: ' + error.message);
                }
            });
        },
        // 自动检测提示词节点
        handleAutoDetect() {
            if (!this.workflowForm.workflow_json) {
                ElMessage.warning('请先输入工作流JSON');
                return;
            }

            try {
                const workflow = JSON.parse(this.workflowForm.workflow_json);
                let positiveNodeId = null;
                let negativeNodeId = null;

                for (const [nodeId, node] of Object.entries(workflow)) {
                    if (node.class_type === 'CLIPTextEncode') {
                        if (!positiveNodeId) {
                            positiveNodeId = nodeId;
                        } else if (!negativeNodeId) {
                            negativeNodeId = nodeId;
                            break;
                        }
                    }
                }

                if (positiveNodeId) {
                    this.workflowForm.positive_node_id = positiveNodeId;
                    this.workflowForm.positive_field_name = 'text';
                    ElMessage.success('已检测到正向提示词节点');
                }
                if (negativeNodeId) {
                    this.workflowForm.negative_node_id = negativeNodeId;
                    this.workflowForm.negative_field_name = 'text';
                    ElMessage.success('已检测到负向提示词节点');
                }
                if (!positiveNodeId && !negativeNodeId) {
                    ElMessage.warning('未检测到提示词节点');
                }
            } catch (error) {
                ElMessage.error('解析工作流JSON失败: ' + error.message);
            }
        },
        // 从文件加载
        handleLoadFromFile() {
            const input = document.createElement('input');
            input.type = 'file';
            input.accept = '.json';
            input.onchange = (e) => {
                const file = e.target.files[0];
                if (file) {
                    const reader = new FileReader();
                    reader.onload = (event) => {
                        try {
                            const content = event.target.result;
                            const json = JSON.parse(content);
                            this.workflowForm.workflow_json = this.formatJSON(content);
                            ElMessage.success('文件加载成功');
                        } catch (error) {
                            ElMessage.error('文件格式错误: ' + error.message);
                        }
                    };
                    reader.readAsText(file);
                }
            };
            input.click();
        },
        // 格式化JSON
        handleFormatJSON() {
            if (!this.workflowForm.workflow_json) {
                ElMessage.warning('请先输入JSON内容');
                return;
            }
            try {
                this.workflowForm.workflow_json = this.formatJSON(this.workflowForm.workflow_json);
                ElMessage.success('格式化成功');
            } catch (error) {
                ElMessage.error('JSON格式错误: ' + error.message);
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
        formatJSON(jsonString) {
            try {
                const obj = JSON.parse(jsonString);
                return JSON.stringify(obj, null, 2);
            } catch (e) {
                return jsonString;
            }
        },
        // 获取节点数量
        getNodeCount(workflow) {
            if (!workflow.workflow_json) return 0;
            try {
                const wf = JSON.parse(workflow.workflow_json);
                return Object.keys(wf).length;
            } catch (e) {
                return 0;
            }
        },
        // 解析标签
        getTags(tagsStr) {
            if (!tagsStr) return [];
            return tagsStr.split(',').map(tag => tag.trim()).filter(tag => tag);
        }
    }
});

app.use(ElementPlus);
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
    app.component(key, component);
}
app.mount('#app');

