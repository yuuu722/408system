<template> 
  <div class="app-container"> 
    <!-- 侧边栏 --> 
    <aside class="sidebar"> 
      <div class="logo"> 
        <h2>408考研问答</h2> 
        <p>知识图谱智能辅导</p> 
      </div> 
      <nav class="subject-nav"> 
        <div 
          v-for="sub in subjects" 
          :key="sub.id" 
          class="subject-item" 
          :class="{ active: activeSubject === sub.id }" 
          @click="switchSubject(sub.id)" 
        > 
          {{ sub.name }} 
        </div> 
      </nav> 
      <div class="graph-toggle" @click="openKnowledgeBase"> 
        📚 知识库管理 
      </div> 
      <div class="graph-toggle" @click="showKnowledgeBase = false; showGraph = !showGraph"> 
        {{ showGraph ? '隐藏知识图谱' : '查看知识图谱' }} 
      </div> 
    </aside> 
 
    <!-- 主内容区 --> 
    <main class="main-content"> 
      <!-- 知识库管理面板 --> 
      <div v-if="showKnowledgeBase" class="knowledge-panel"> 
        <div class="knowledge-header"> 
          <div> 
            <h2>408知识库</h2> 
            <p>上传教材、笔记和408学习资料，为后续RAG检索提供知识来源。</p> 
          </div> 
 
          <button class="close-button" @click="showKnowledgeBase = false"> 
            关闭 
          </button> 
        </div> 
 
        <div class="upload-box"> 
          <input 
            ref="fileInput" 
            type="file" 
            accept=".pdf,.txt,.md,.docx" 
            @change="handleFileChange" 
          /> 
 
          <button 
            class="upload-button" 
            :disabled="!selectedFile || uploading" 
            @click="uploadKnowledgeFile" 
          > 
            {{ uploading ? '上传中...' : '上传资料' }} 
          </button> 
        </div> 
 
        <div v-if="uploadMessage" class="upload-message"> 
          {{ uploadMessage }} 
        </div> 
 
        <div class="knowledge-files"> 
          <h3>已上传资料</h3> 
 
          <div v-if="knowledgeFiles.length === 0" class="empty-files"> 
            暂无知识库资料 
          </div> 
 
          <div 
            v-for="file in knowledgeFiles" 
            :key="file.filename" 
            class="knowledge-file" 
          > 
            <span>📄 {{ file.filename }}</span> 
            <span>{{ formatFileSize(file.size) }}</span> 
          </div> 
        </div> 
      </div> 

      <!-- 知识图谱面板 --> 
      <div v-if="showGraph" class="graph-panel"> 
        <div ref="graphChart" class="graph-chart"></div> 
      </div> 
 
      <!-- 聊天区域 --> 
      <div class="chat-area"> 
        <div class="messages" ref="messagesContainer"> 
          <div v-if="messages.length === 0" class="welcome"> 
            <h1>你好，我是408考研智能辅导助手</h1> 
            <p>我可以帮你解答数据结构、计算机组成原理、操作系统、计算机网络的相关问题</p> 
            <div class="quick-questions"> 
              <div 
                v-for="q in quickQuestions" 
                :key="q" 
                class="quick-question" 
                @click="sendQuestion(q)" 
              > 
                {{ q }} 
              </div> 
            </div> 
          </div> 
 
          <div 
            v-for="(msg, idx) in messages" 
            :key="idx" 
            class="message" 
            :class="msg.role" 
          > 
            <div class="message-content"> 
              <div class="message-text" v-html="formatContent(msg.content)"></div> 
              <div v-if="msg.evidence && msg.evidence.length" class="evidence"> 
                <div class="evidence-header" @click="showEvidence[idx] = !showEvidence[idx]"> 
                  📚 参考依据 ({{ msg.evidence.length }}) 
                  <span class="arrow">{{ showEvidence[idx] ? '▼' : '▶' }}</span> 
                </div> 
                <div v-if="showEvidence[idx]" class="evidence-list"> 
                  <div v-for="(ev, i) in msg.evidence" :key="i" class="evidence-item"> 
                    <div class="evidence-title">[{{ i + 1 }}] {{ ev.title }}</div> 
                    <div class="evidence-source">来源：{{ ev.source }}</div> 
                  </div> 
                </div> 
              </div> 
            </div> 
          </div> 
 
          <div v-if="isTyping" class="message assistant"> 
            <div class="message-content typing"> 
              <span></span><span></span><span></span> 
            </div> 
          </div> 
        </div> 
 
        <!-- 输入框 --> 
        <div class="input-area"> 
          <div class="input-wrapper"> 
            <textarea 
              v-model="inputText" 
              placeholder="输入你的问题..." 
              @keydown.enter.prevent="handleEnter" 
              rows="1" 
            ></textarea> 
            <button @click="sendQuestion(inputText)" :disabled="!inputText.trim()"> 
              发送 
            </button> 
          </div> 
        </div> 
      </div> 
    </main> 
  </div> 
</template> 
 
<script> 
import { $echarts } from './echarts-setup' 
 
export default { 
  name: 'App', 
  data() { 
    return { 
      subjects: [ 
        { id: 'ds', name: '数据结构' }, 
        { id: 'co', name: '计算机组成原理' }, 
        { id: 'os', name: '操作系统' }, 
        { id: 'cn', name: '计算机网络' }, 
      ], 
      activeSubject: 'all', 
      showGraph: false, 
      showKnowledgeBase: false, 
      selectedFile: null, 
      uploading: false, 
      uploadMessage: '', 
      knowledgeFiles: [], 
      messages: [], 
      inputText: '', 
      isTyping: false, 
      showEvidence: {}, 
      quickQuestions: [ 
        '数组和链表有什么区别？', 
        '什么是进程和线程？', 
        'TCP和UDP的区别是什么？', 
        '栈和队列的特点是什么？', 
      ], 
      sessionId: 'session_' + Date.now(), 
      graphChart: null, 
    } 
  }, 
  mounted() { 
    if (this.showGraph) { 
      this.loadGraph() 
    } 
  }, 
  methods: { 
    switchSubject(id) { 
      this.activeSubject = id 
      if (this.showGraph) { 
        this.loadGraph() 
      } 
    }, 
    async openKnowledgeBase() { 
      this.showKnowledgeBase = true 
      this.showGraph = false 
      await this.loadKnowledgeFiles() 
    }, 
    handleFileChange(event) { 
      this.selectedFile = event.target.files[0] || null 
      this.uploadMessage = '' 
    }, 
    async uploadKnowledgeFile() { 
      if (!this.selectedFile) return 
 
      this.uploading = true 
      this.uploadMessage = '' 
 
      try { 
        const formData = new FormData() 
        formData.append('file', this.selectedFile) 
 
        const response = await fetch('/api/knowledge/upload', { 
          method: 'POST', 
          body: formData 
        }) 
 
        const data = await response.json() 
 
        if (!response.ok) { 
          throw new Error(data.detail || '上传失败') 
        } 
 
        this.uploadMessage = `上传成功：${data.filename}` 
        this.selectedFile = null 
 
        if (this.$refs.fileInput) { 
          this.$refs.fileInput.value = '' 
        } 
 
        await this.loadKnowledgeFiles() 
      } catch (e) { 
        this.uploadMessage = '上传失败：' + e.message 
      } finally { 
        this.uploading = false 
      } 
    }, 
    async loadKnowledgeFiles() { 
      try { 
        const response = await fetch('/api/knowledge/files') 
        const data = await response.json() 
 
        if (data.success) { 
          this.knowledgeFiles = data.files || [] 
        } 
      } catch (e) { 
        console.error('加载知识库文件失败:', e) 
      } 
    }, 
    formatFileSize(size) { 
      if (size < 1024) { 
        return size + ' B' 
      } else if (size < 1024 * 1024) { 
        return (size / 1024).toFixed(1) + ' KB' 
      } else { 
        return (size / 1024 / 1024).toFixed(1) + ' MB' 
      } 
    }, 
    async sendQuestion(question) { 
      if (!question || !question.trim()) return 
      this.inputText = '' 
 
      // 添加用户消息 
      this.messages.push({ role: 'user', content: question }) 
      this.scrollToBottom() 
 
      this.isTyping = true 
 
      try { 
        const response = await fetch('/api/chat', { 
          method: 'POST', 
          headers: { 'Content-Type': 'application/json' }, 
          body: JSON.stringify({ 
            query: question, 
            history: this.messages.slice(0, -1).map(m => ({ 
              role: m.role, 
              content: m.content 
            })), 
            session_id: this.sessionId 
          }) 
        }) 
 
        const reader = response.body.getReader() 
        const decoder = new TextDecoder() 
        let assistantContent = '' 
        let evidence = [] 
        let messageIdx = this.messages.length 
 
        this.messages.push({ role: 'assistant', content: '', evidence: [] }) 
 
        while (true) { 
          const { done, value } = await reader.read() 
          if (done) break 
 
          const lines = decoder.decode(value).split('\n\n') 
          for (const line of lines) { 
            if (!line.startsWith('data: ')) continue 
            try { 
              const data = JSON.parse(line.slice(6)) 
              if (data.type === 'token') { 
                assistantContent += data.data 
                this.messages[messageIdx].content = assistantContent 
                this.scrollToBottom() 
              } else if (data.type === 'evidence') { 
                evidence = data.data 
                this.messages[messageIdx].evidence = evidence 
              } else if (data.type === 'error') { 
                this.messages[messageIdx].content = '出错了：' + data.data 
              } 
            } catch (e) { 
              console.error('解析SSE数据失败:', e) 
            } 
          } 
        } 
 
        this.showEvidence[messageIdx] = false 
 
      } catch (e) { 
        this.messages.push({ 
          role: 'assistant', 
          content: '网络错误，请检查后端服务是否启动。' 
        }) 
      } finally { 
        this.isTyping = false 
        this.scrollToBottom() 
      } 
    }, 
    handleEnter(e) { 
      if (!e.shiftKey) { 
        this.sendQuestion(this.inputText) 
      } 
    }, 
    formatContent(text) { 
      // 简单的 Markdown 渲染 
      return text 
        .replace(/\n/g, '<br>') 
        .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>') 
        .replace(/\*(.*?)\*/g, '<em>$1</em>') 
    }, 
    scrollToBottom() { 
      this.$nextTick(() => { 
        const container = this.$refs.messagesContainer 
        if (container) { 
          container.scrollTop = container.scrollHeight 
        } 
      }) 
    }, 
    async loadGraph() { 
      try { 
        const url = this.activeSubject === 'all' 
          ? '/api/knowledge-graph' 
          : `/api/knowledge-graph?subject=${this.activeSubject}` 
 
        const res = await fetch(url) 
        const data = await res.json() 
        this.renderGraph(data) 
      } catch (e) { 
        console.error('加载知识图谱失败:', e) 
      } 
    }, 
    renderGraph(data) { 
      if (!this.$refs.graphChart) return 
 
      if (this.graphChart) { 
        this.graphChart.dispose() 
      } 
 
      this.graphChart = $echarts.init(this.$refs.graphChart) 
 
      const colors = { 
        ds: '#409EFF', 
        co: '#67C23A', 
        os: '#E6A23C', 
        cn: '#F56C6C', 
      } 
 
      const nodes = data.nodes.map(n => ({ 
        id: n.id, 
        name: n.label, 
        symbolSize: 40, 
        itemStyle: { 
          color: colors[n.subject] || '#909399' 
        }, 
        label: { show: true } 
      })) 
 
      const links = data.edges.map(e => ({ 
        source: e.source, 
        target: e.target, 
        label: { show: true, formatter: e.relation }, 
      })) 
 
      const option = { 
        title: { text: '408知识图谱', left: 'center' }, 
        tooltip: {}, 
        series: [{ 
          type: 'graph', 
          layout: 'force', 
          data: nodes, 
          links: links, 
          roam: true, 
          force: { repulsion: 200, edgeLength: 100 } 
        }] 
      } 
 
      this.graphChart.setOption(option) 
    } 
  }, 
  watch: { 
    showGraph(val) { 
      if (val) { 
        setTimeout(() => this.loadGraph(), 100) 
      } 
    } 
  } 
} 
</script> 
 
<style src="./style.css"></style>