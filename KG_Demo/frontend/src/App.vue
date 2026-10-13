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
      <div class="graph-toggle" @click="showGraph = !showGraph">
        {{ showGraph ? '隐藏知识图谱' : '查看知识图谱' }}
      </div>
    </aside>

    <!-- 主内容区 -->
    <main class="main-content">
      <!-- 知识图谱面板 -->
      <div v-if="showGraph" class="graph-panel" :class="{ fullscreen: graphFullscreen }">
        <div ref="graphChart" class="graph-chart"></div>
      </div>

      <!-- 聊天区域 -->
      <div class="chat-area" v-show="!graphFullscreen">
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
      graphFullscreen: false,
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
    // 演示模式（便于展示/截图）：?view=graph 自动打开图谱；?ask=q1..q4 自动提问
    const params = new URLSearchParams(window.location.search)
    if (params.get('view') === 'graph') {
      this.showGraph = true
      this.graphFullscreen = true
      this.$nextTick(() => this.loadGraph())
    }
    const askMap = {
      q1: '数组和链表有什么区别？',
      q2: '什么是进程和线程？',
      q3: 'TCP和UDP的区别是什么？',
      q4: '栈和队列的特点是什么？',
    }
    const askKey = params.get('ask')
    if (askKey && askMap[askKey]) {
      const question = askMap[askKey]
      this.$nextTick(() => this.sendQuestion(question))
    }
  },
  methods: {
    switchSubject(id) {
      this.activeSubject = id
      if (this.showGraph) {
        this.loadGraph()
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
        let buffer = ''

        this.messages.push({ role: 'assistant', content: '', evidence: [] })

        const handleLine = (line) => {
          if (!line.startsWith('data: ')) return
          const data = JSON.parse(line.slice(6))
          if (data.type === 'token') {
            assistantContent += data.data
            this.messages[messageIdx].content = assistantContent
            this.scrollToBottom()
          } else if (data.type === 'evidence') {
            evidence = data.data
            this.messages[messageIdx].evidence = evidence
          } else if (data.type === 'cached') {
            assistantContent = data.data.answer || ''
            evidence = data.data.evidence || []
            this.messages[messageIdx].content = assistantContent
            this.messages[messageIdx].evidence = evidence
            this.scrollToBottom()
          } else if (data.type === 'error') {
            this.messages[messageIdx].content = '出错了：' + data.data
          }
        }

        while (true) {
          const { done, value } = await reader.read()
          if (done) break

          buffer += decoder.decode(value, { stream: true })
          const parts = buffer.split('\n\n')
          buffer = parts.pop()  // 末尾可能是不完整事件，留待下一块拼接
          for (const line of parts) {
            try { handleLine(line.trim()) } catch (e) {
              console.error('解析SSE数据失败:', e)
            }
          }
        }

        if (buffer.trim()) {
          try { handleLine(buffer.trim()) } catch (e) {
            console.error('解析SSE数据失败:', e)
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
        symbolSize: 46,
        itemStyle: {
          color: colors[n.subject] || '#909399'
        },
        label: { show: true, fontSize: 13 }
      }))

      const links = data.edges.map(e => ({
        source: e.source,
        target: e.target,
        label: { show: true, formatter: e.relation, fontSize: 11, color: '#909399' },
      }))

      const option = {
        title: { text: '408知识图谱', left: 'center', top: 12, textStyle: { fontSize: 20 } },
        tooltip: {},
        series: [{
          type: 'graph',
          layout: 'force',
          top: 56,
          bottom: 24,
          data: nodes,
          links: links,
          roam: true,
          label: { position: 'right' },
          force: { repulsion: 520, edgeLength: 115, gravity: 0.08 }
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
