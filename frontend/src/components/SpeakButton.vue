<script setup lang="ts">
import {onMounted,onBeforeUnmount,ref} from 'vue'
const props=defineProps<{text:string}>(),voices=ref<SpeechSynthesisVoice[]>([]),selected=ref(''),speaking=ref(false),error=ref('')
const available=typeof window!=='undefined'&&'speechSynthesis' in window
function refresh(){voices.value=window.speechSynthesis.getVoices().filter(v=>/^zh/i.test(v.lang));if(!selected.value)selected.value=voices.value[0]?.voiceURI??''}
function stop(){if(available)window.speechSynthesis.cancel();speaking.value=false}
function speak(){stop();error.value='';if(!available)return;const text=props.text.replace(/[#*`]/g,'').trim();if(!text)return
 const parts=text.match(/[^。！？\n]+[。！？\n]?/g)??[text];let n=0;speaking.value=true
 function next(){if(!speaking.value)return;const part=parts[n++];if(!part){speaking.value=false;return};const u=new SpeechSynthesisUtterance(part);u.lang='zh-CN';u.rate=.95;u.voice=voices.value.find(v=>v.voiceURI===selected.value)??null;u.onend=next;u.onerror=()=>{speaking.value=false;error.value='朗读未完成，请检查设备中文语音支持。'};window.speechSynthesis.speak(u)}next()
}
onMounted(()=>{if(available){refresh();window.speechSynthesis.addEventListener('voiceschanged',refresh)}})
onBeforeUnmount(()=>{if(available){window.speechSynthesis.removeEventListener('voiceschanged',refresh);stop()}})
</script>
<template><span class="voice-control"><button v-if="available" type="button" class="secondary" @click="speaking?stop():speak()">{{speaking?'停止朗读':'▶ 语音讲解'}}</button><select v-if="voices.length>1" v-model="selected" aria-label="选择中文音色"><option v-for="v in voices" :key="v.voiceURI" :value="v.voiceURI">{{v.name}}</option></select><small v-if="!available">此浏览器不支持语音朗读</small><small v-else-if="!voices.length">使用设备默认语音；可安装中文音色</small><small v-if="error" role="alert">{{error}}</small></span></template>
<style scoped>.voice-control{display:inline-flex;gap:8px;align-items:center;flex-wrap:wrap}.voice-control select{max-width:220px}.voice-control small{color:#64748b}</style>
